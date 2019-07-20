#!/usr/bin/env bash

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
DATA_DIR=/tmp/k8s
GO_OUT="${KUBE_ROOT}/_output/bin"
LOAD_BALANCER_PID=
LOAD_BALANCER_DNS="k8s"
LOAD_BALANCER_IP="10.1.1.1"
LOAD_BALANCER_PORT="6440"
APISERVER_PIDS=""
SCHEDULER_PIDS=""
CONTROLLER_MANAGER_PIDS=""
KUBELET_PID=""
N_MASTERS=1
LOG_LEVEL=${LOG_LEVEL:-3}

KUBEADM="${GO_OUT}/kubeadm"
KUBECTL="${GO_OUT}/kubectl"
HYPERKUBE="${GO_OUT}/hyperkube"

# Build flags
GOFLAGS="-tags=libsqlite3"
CGO_OVERRIDES="kube-apiserver"

# Stop right away if the build fails
set -e

source "${KUBE_ROOT}/hack/lib/init.sh"

function start_load_balancer {
    HAPROXY_CFG="${DATA_DIR}/haproxy.cfg"

    if ! ip address show dev k8s > /dev/null 2>&1; then
	sudo ip link add k8s up type bridge
	sudo ip addr add ${LOAD_BALANCER_IP}/16 dev k8s
    fi

    cat > ${HAPROXY_CFG} <<EOF
defaults
	log	global
	mode	tcp
        timeout connect 500000
        timeout client  500000
        timeout server  500000

frontend k8s
	bind ${LOAD_BALANCER_IP}:${LOAD_BALANCER_PORT}
	default_backend apiserver

backend apiserver
EOF
    for ID in $(seq ${N_MASTERS}); do
	cat >> ${HAPROXY_CFG} <<EOF
	server apiserver-${ID} ${LOAD_BALANCER_IP}:644${ID} check fall 1 rise 2
EOF
    done

    haproxy -q -f ${HAPROXY_CFG} &
    LOAD_BALANCER_PID=$!
}

function start_apiserver {
    ID="$1"
    NODE_DIR="${DATA_DIR}/${ID}"
    STORAGE_DIR="${NODE_DIR}/backend"
    KUBEADM_CONF="${NODE_DIR}/kubeadm.conf"
    CERT_DIR="${NODE_DIR}/certs"
    SECURE_PORT="644${ID}"
    INSECURE_PORT="808${ID}"
    LOG="${NODE_DIR}/apiserver.log"

    mkdir -p ${NODE_DIR}
    mkdir -p ${CERT_DIR}

    if ! [ -f "${CERT_DIR}/ca.crt" ]; then
	cat > ${KUBEADM_CONF} <<EOF
apiVersion: kubeadm.k8s.io/v1beta2
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: ${LOAD_BALANCER_IP}
  bindPort: 644${ID}
---
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
kubernetesVersion: stable
controlPlaneEndpoint: "${LOAD_BALANCER_DNS}:${LOAD_BALANCER_PORT}"
certificatesDir: ${CERT_DIR}
EOF
	if [ "$ID" == "1" ]; then
	    ${KUBEADM} init phase certs ca --config=${KUBEADM_CONF}
	    ${KUBEADM} init phase certs apiserver --config=${KUBEADM_CONF}
	    # Use openssl to generate the service account key, since kubeadm doesn't
	    # accept the --config option for this subcommand.
	    #${KUBEADM} init phase certs sa --cert-dir=${CERT_DIR}
	    openssl genrsa -out ${CERT_DIR}/sa.key 2048 2>/dev/null
	    ${KUBEADM} init phase kubeconfig kubelet --config=${KUBEADM_CONF} --kubeconfig-dir=${DATA_DIR}
	    ${KUBEADM} init phase kubeconfig admin --config=${KUBEADM_CONF} --kubeconfig-dir=${DATA_DIR}
	    ${KUBECTL} dqlite bootstrap --id 1 --address 127.0.0.1:9001 --dir ${STORAGE_DIR}
	else
	    BOOTSTRAP_CERT_DIR="${DATA_DIR}/1/certs"
	    CERT_FILES="ca.key ca.crt sa.key"
	    for f in ${CERT_FILES}; do
		cp ${BOOTSTRAP_CERT_DIR}/${f} ${CERT_DIR}/${f}
	    done
	    SEP=""
	    CLUSTER=""
	    for OTHER_ID in $(seq $(($ID - 1))); do
		CLUSTER="${CLUSTER}${SEP}127.0.0.1:900${OTHER_ID}"
		SEP=","
	    done
	    ${KUBECTL} dqlite join --id ${ID} --address 127.0.0.1:900${ID} --dir ${STORAGE_DIR} --cluster ${CLUSTER}
	fi
	${KUBEADM} init phase certs apiserver --config=${KUBEADM_CONF}
	${KUBEADM} init phase kubeconfig scheduler --config=${KUBEADM_CONF} --kubeconfig-dir=${NODE_DIR}
	${KUBEADM} init phase kubeconfig controller-manager --config=${KUBEADM_CONF} --kubeconfig-dir=${NODE_DIR}
    fi

    $HYPERKUBE kube-apiserver \
	       --v=${LOG_LEVEL} \
	       --authorization-mode=Node,RBAC \
	       --client-ca-file=${CERT_DIR}/ca.crt \
	       --storage-dir=${STORAGE_DIR} \
	       --cert-dir=${CERT_DIR} \
	       --advertise-address=${LOAD_BALANCER_IP} \
	       --bind-address=${LOAD_BALANCER_IP} \
	       --secure-port=${SECURE_PORT} \
	       --insecure-bind-address=127.0.0.1 \
	       --insecure-port=${INSECURE_PORT} \
	       --tls-cert-file=${CERT_DIR}/apiserver.crt \
	       --tls-private-key-file=${CERT_DIR}/apiserver.key \
	       --storage-backend=dqlite \
	       --service-account-key-file=${CERT_DIR}/sa.key \
	       --service-account-lookup=true \
	       --apiserver-count=${N_MASTERS} \
	       --endpoint-reconciler-type="master-count" \
	       --external-hostname=${LOAD_BALANCER_DNS} > ${LOG} 2>&1 &
    if [ "$ID" == "1" ]; then
	APISERVER_PIDS="$!"
    else
	APISERVER_PIDS="${APISERVER_PIDS} ${!}"
    fi
}

function start_controller_manager {
    ID="$1"
    NODE_DIR="${DATA_DIR}/${ID}"
    CERT_DIR="${NODE_DIR}/certs"
    SECURE_PORT="${ID}0257"
    INSECURE_PORT="${ID}0252"
    LOG="${NODE_DIR}/controller-manager.log"

    $HYPERKUBE kube-controller-manager \
	       --v=${LOG_LEVEL} \
	       --service-account-private-key-file=${CERT_DIR}/sa.key \
	       --root-ca-file=${CERT_DIR}/ca.crt \
	       --cluster-signing-cert-file=${CERT_DIR}/ca.crt \
	       --cluster-signing-key-file=${CERT_DIR}/ca.key \
	       --secure-port=${SECURE_PORT} \
	       --port=${INSECURE_PORT} \
	       --leader-elect-lease-duration=20s \
	       --leader-elect-renew-deadline=15s \
	       --leader-elect-retry-period=4s \
	       --kubeconfig=${NODE_DIR}/controller-manager.conf \
	       --use-service-account-credentials \
	       --cert-dir=${CERT_DIR} > ${LOG} 2>&1 &
    if [ "$ID" == "1" ]; then
	CONTROLLER_MANAGER_PIDS="$!"
    else
	CONTROLLER_MANAGER_PIDS="${CONTROLLER_MANAGER_PIDS} ${!}"
    fi
}

function start_scheduler {
    ID="$1"
    NODE_DIR="${DATA_DIR}/${ID}"
    CERT_DIR="${NODE_DIR}/certs"
    SECURE_PORT="${ID}0259"
    INSECURE_PORT="${ID}0251"
    LOG="${NODE_DIR}/scheduler.log"

    $HYPERKUBE kube-scheduler \
	       --v=${LOG_LEVEL} \
	       --leader-elect-lease-duration=20s \
	       --leader-elect-renew-deadline=15s \
	       --leader-elect-retry-period=4s \
	       --secure-port=${SECURE_PORT} \
	       --port=${INSECURE_PORT} \
	       --kubeconfig="${NODE_DIR}/scheduler.conf" > ${LOG} 2>&1 &
    if [ "$ID" == "1" ]; then
	SCHEDULER_PIDS="$!"
    else
	SCHEDULER_PIDS="${SCHEDULER_PIDS} ${!}"
    fi
}

function start_controlplane {
    # Start the control plane components on each master.
    for ID in $(seq ${N_MASTERS}); do
	start_apiserver $ID
	start_scheduler $ID
	start_controller_manager $ID
	sleep 2
    done

    # Wait for the cluster to become available
    kube::util::wait_for_url "https://${LOAD_BALANCER_IP}:${LOAD_BALANCER_PORT}/healthz" "apiserver: " 1 10 1

    # Grant apiserver permission to speak to the kubelet
    ${KUBECTL} --kubeconfig ${DATA_DIR}/admin.conf create clusterrolebinding kube-apiserver-kubelet-admin --clusterrole=system:kubelet-api-admin --user=kube-apiserver

    # Create storage
    ${KUBECTL} --kubeconfig=${DATA_DIR}/admin.conf create -f ${KUBE_ROOT}/cluster/addons/storage-class/local/default.yaml
}

function start_kubelet {
    DOCKER_DIR="${DATA_DIR}/docker"
    LOG="${DATA_DIR}/kubelet.log"

    kube::docker::start

    sudo -E $HYPERKUBE kubelet \
	 --v=${LOG_LEVEL} \
	 --address=127.0.0.1 \
	 --kubeconfig=${DATA_DIR}/kubelet.conf \
	 --client-ca-file=${DATA_DIR}/1/certs/ca.crt \
	 --docker-endpoint=unix://${DOCKER_DIR}/socket \
	 --fail-swap-on=false \
	 --hostname-override="localhost" > ${LOG} 2>&1 &
    KUBELET_PID=$!
}

function cleanup
{
    if [ -n "${LOAD_BALANCER_PID}" ]; then
        kill -SIGUSR1 ${LOAD_BALANCER_PID}
    fi
    if [ -n "${APISERVER_PIDS}" ]; then
	for pid in ${APISERVER_PIDS}; do
            if kill -SIGINT ${pid}; then
		echo killed apiserver ${pid}
	    fi
	done
    fi
    if [ -n "${SCHEDULER_PIDS}" ]; then
	for pid in ${SCHEDULER_PIDS}; do
            if kill -SIGTERM ${pid}; then
		echo killed scheduler ${pid}
	    fi
	done
    fi
    if [ -n "${CONTROLLER_MANAGER_PIDS}" ]; then
	for pid in ${CONTROLLER_MANAGER_PIDS}; do
            if kill -SIGTERM ${pid}; then
		echo killed controller manager ${pid}
	    fi
	done
    fi
    if [ -n "${KUBELET_PID}" ]; then
        if sudo kill -SIGTERM ${KUBELET_PID}; then
	    echo killed kubelet ${KUBELET_PID}
	fi
    fi
    kube::docker::stop
    kube::util::wait-for-jobs || true

    exit 0
}

kube::util::ensure-gnu-sed
kube::util::test_openssl_installed
kube::util::ensure-cfssl

trap cleanup EXIT

#make -C "${KUBE_ROOT}" WHAT="cmd/kubeadm cmd/kubectl cmd/hyperkube" GOFLAGS=$GOFLAGS KUBE_CGO_OVERRIDES=$CGO_OVERRIDES

mkdir -p ${DATA_DIR}

start_load_balancer
start_controlplane
start_kubelet

while true; do sleep 1 || true; done
