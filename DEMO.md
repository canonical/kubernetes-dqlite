# Product roadmap sprint demo

With respect to rancher's k3s (based on SQLite) this dqlite-based backend
supports load-balancing of API requests and HA with automatic failover. It won't
be as scalable as the etcd backend because watchers are still all served by the
dqlite leader. Further work around scalability should be probably be started
only when real-world users start to hit some walls.

I didn't look into details, but other SQL backend supported by rancher
(e.g. PostgreSQL and MySQL) might support some form of HA (with or without
automatic failover), but currently don't support load balancing. And of course
they require you to operate the backend SQL service separately from k8s, as for
the etcd case.

The demo will roughtly consist of:

- Pre-demo preparation:
  - Start 5 machines running Ubuntu 18.04 (preferably on a reliably substrate such as local VMs or stgraber's MAAS)
  - Deploy HAProxy on machine 0.
  - Deploy k8s an HA control plane on machine 1, 2 and 3. Machine 1 will be the initial leader.
  - Deploy a k8s worker (kubelet) on machine 4.
  - Deploy a sample workload which will be scheduled on machine 4.

and then:

- The demo itself:
  - Stop the kube-apiserver component on machine 1.
  - Observe that kubectl continues to work.
  - Observe that the other 2 control plane services running on machine 1 (kube-scheduler
    and kube-controller-manager) maintain their leader status, since they contact the
    other two kube-apiserver instances in order to renew their leases.
  - Stop the kube-scheduler and kube-controller-management components on machine 1.
  - Observe that a leadership change occurs both for the scheduler and the controller.
  - Delete the sample workload and observe that it's properly shutdown.

Note that all the boilerplate and configuration steps described below to setup
the cluster are only needed in this early phase. They should be automated once we
settle on a certain user experience and implement it in either ```microk8s``` or
```CDK```, or even the native ```kubeadm```.

## Deploy HAProxy on machine 0

This creates a load-balanced endpoint which round-robins requests across the 3
control plane machines.

Run:

```bash
sudo apt-get update
sudo apt-get install haproxy
LOAD_BALANCER_IP=<IP of machine 0>
API_SERVER_1_IP=<IP of machine 1>
API_SERVER_2_IP=<IP of machine 2>
API_SERVER_3_IP=<IP of machine 3>
sudo -E cat | sudo tee /etc/haproxy/haproxy.cfg > /dev/null <<EOF
defaults
  log  global
  mode tcp
  timeout connect 500000
  timeout client  500000
  timeout server  500000

frontend k8s
  bind ${LOAD_BALANCER_IP}:6443
  default_backend apiserver

backend apiserver
  server apiserver-1 ${API_SERVER_1_IP}:6443 check fall 1 rise 2
  server apiserver-2 ${API_SERVER_2_IP}:6443 check fall 1 rise 2
  server apiserver-3 ${API_SERVER_3_IP}:6443 check fall 1 rise 2
EOF
sudo systemctl restart haproxy
```

## Build k8s on machines 1, 2, 3 and 4

This builds the patched k8s binaries.

***NB***: Keep the environment variables defined here also when running
          the commands in the other sections below.

Run.

```bash
sudo apt-get update
sudo apt-get install -y tcl-dev libtool autoconf pkg-config libuv1-dev sqlite3
sudo snap install go --classic
git clone --depth 5 https://github.com/lxc/lxd.git
cd lxd
make deps
export CGO_CFLAGS="-I/home/ubuntu/go/deps/sqlite/ -I/home/ubuntu/go/deps/libco/ -I/home/ubuntu/go/deps/raft/include/ -I/home/ubuntu/go/deps/dqlite/include/"
export CGO_LDFLAGS="-L/home/ubuntu/go/deps/sqlite/.libs/ -L/home/ubuntu/go/deps/libco/ -L/home/ubuntu/go/deps/raft/.libs -L/home/ubuntu/go/deps/dqlite/.libs/"
export LD_LIBRARY_PATH="/home/ubuntu/go/deps/sqlite/.libs/:/home/ubuntu/go/deps/libco/:/home/ubuntu/go/deps/raft/.libs/:/home/ubuntu/go/deps/dqlite/.libs/"
cd ..
git clone -b dqlite-backend --depth 100 https://github.com/freeekanayaka/kubernetes.git
cd kubernetes
make WHAT="cmd/kubeadm cmd/kubectl cmd/hyperkube" GOFLAGS=-tags=libsqlite3 KUBE_CGO_OVERRIDES=kube-apiserver
sudo cp _output/bin/* /usr/bin/
cd ..
mkdir -p go/src
cd go/src
ln -s ../../kubernetes/vendor/github.com/ .
ln -s ../../kubernetes/vendor/golang.org/ .
ln -s ../../kubernetes/vendor/google.golang.org/ .
ln -s ../../kubernetes/vendor/gopkg.in/ .
ln -s ../../kubernetes/vendor/k8s.io/ .
ln -s ../../kubernetes/vendor/sigs.k8s.io/ .
cd github.com/freeekanayaka
git clone https://github.com/freeekanayaka/kubectl-dqlite.git
cd kubectl-dqlite
go build -tags libsqlite3 -o kubectl-dqlite ./cmd/kubectl-dqlite.go
sudo cp kubectl-dqlite /usr/bin
cd ~
```

## Common system configuration on machines 1, 2, and 3

This creates some directories, configuration files and systemd units.

Run:

```bash
cd ~
mkdir certs
LOAD_BALANCER_IP=<IP of machine 0>
LOAD_BALANCER_DNS=<DNS name of machine 0>
LOAD_BALANCER_PORT=6443
CERT_DIR=/home/ubuntu/certs
STORAGE_DIR=/home/ubuntu/backend
KUBEADM_CONF=/home/ubuntu/kubeadm.conf
API_SERVER_1_IP=<IP of machine 1>
API_SERVER_2_IP=<IP of machine 2>
API_SERVER_3_IP=<IP of machine 3>
cat > ${KUBEADM_CONF} <<EOF
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
kubernetesVersion: stable
controlPlaneEndpoint: ${LOAD_BALANCER_DNS}:${LOAD_BALANCER_PORT}
certificatesDir: ${CERT_DIR}
EOF

mkdir ${CERT_DIR}
sudo mkdir /etc/kubernetes

sudo -E cat | sudo tee /etc/kubernetes/config > /dev/null <<EOF
LOG_LEVEL=3
CERT_DIR=/home/ubuntu/certs
EOF

sudo -E cat | sudo tee /etc/kubernetes/apiserver > /dev/null <<EOF
STORAGE_DIR=/home/ubuntu/backend
LOAD_BALANCER_DNS=${LOAD_BALANCER_DNS}
EOF

sudo -E cat | sudo tee /etc/systemd/system/kube-apiserver.service > /dev/null <<EOF
[Unit]
Description=Kubernetes API Server
After=network.target

[Service]
EnvironmentFile=-/etc/kubernetes/config
EnvironmentFile=-/etc/kubernetes/apiserver
Environment="LD_LIBRARY_PATH=/home/ubuntu/go/deps/sqlite/.libs/:/home/ubuntu/go/deps/libco/:/home/ubuntu/go/deps/raft/.libs/:/home/ubuntu/go/deps/dqlite/.libs/"
User=ubuntu
ExecStart=/usr/bin/hyperkube kube-apiserver \
               --v=\${LOG_LEVEL} \
               --authorization-mode=Node,RBAC \
               --client-ca-file=\${CERT_DIR}/ca.crt \
               --storage-dir=\${STORAGE_DIR} \
               --cert-dir=\${CERT_DIR} \
               --enable-admission-plugins=NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,Priority,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota \
               --feature-gates=AllAlpha=false \
               --tls-cert-file=\${CERT_DIR}/apiserver.crt \
               --tls-private-key-file=\${CERT_DIR}/apiserver.key \
               --storage-backend=dqlite \
               --service-account-key-file=\${CERT_DIR}/sa.key \
               --service-account-lookup=true \
               --kubelet-client-certificate=\${CERT_DIR}/apiserver-kubelet-client.crt \
               --kubelet-client-key=\${CERT_DIR}/apiserver-kubelet-client.key \
               --apiserver-count=3 \
               --endpoint-reconciler-type="master-count" \
               --external-hostname=\${LOAD_BALANCER_DNS}
Restart=on-failure
Type=notify
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

sudo -E cat | sudo tee /etc/systemd/system/kube-scheduler.service > /dev/null <<EOF
[Unit]
Description=Kubernetes Scheduler Plugin

[Service]
EnvironmentFile=-/etc/kubernetes/config
EnvironmentFile=-/etc/kubernetes/scheduler
Environment="LD_LIBRARY_PATH=/home/ubuntu/go/deps/sqlite/.libs/:/home/ubuntu/go/deps/libco/:/home/ubuntu/go/deps/raft/.libs/:/home/ubuntu/go/deps/dqlite/.libs/"
User=ubuntu
ExecStart=/usr/bin/hyperkube kube-scheduler \
               --v=\${LOG_LEVEL} \
               --leader-elect-lease-duration=20s \
               --leader-elect-renew-deadline=15s \
               --leader-elect-retry-period=4s \
               --kubeconfig=/home/ubuntu/scheduler.conf
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

sudo -E cat | sudo tee /etc/systemd/system/kube-controller-manager.service > /dev/null <<EOF
[Unit]
Description=Kubernetes Controller Manager

[Service]
EnvironmentFile=-/etc/kubernetes/config
EnvironmentFile=-/etc/kubernetes/controller-manager
Environment="LD_LIBRARY_PATH=/home/ubuntu/go/deps/sqlite/.libs/:/home/ubuntu/go/deps/libco/:/home/ubuntu/go/deps/raft/.libs/:/home/ubuntu/go/deps/dqlite/.libs/"
User=ubuntu
ExecStart=/usr/bin/hyperkube kube-controller-manager \
               --v=\${LOG_LEVEL} \
               --service-account-private-key-file=\${CERT_DIR}/sa.key \
               --root-ca-file=\${CERT_DIR}/ca.crt \
               --cluster-signing-cert-file=\${CERT_DIR}/ca.crt \
               --cluster-signing-key-file=\${CERT_DIR}/ca.key \
               --leader-elect-lease-duration=20s \
               --leader-elect-renew-deadline=15s \
               --leader-elect-retry-period=4s \
               --kubeconfig=/home/ubuntu/controller-manager.conf \
               --use-service-account-credentials \
               --cert-dir=\${CERT_DIR}
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
```

## Configuration of control plane specific to machine 1

This will create the cluster certificates and bootstrap the first node.

Run:

```bash
kubeadm init phase certs ca --config=${KUBEADM_CONF}
kubeadm init phase certs sa --cert-dir=${CERT_DIR}
kubeadm init phase kubeconfig kubelet --config=${KUBEADM_CONF} --node-name kubelet --kubeconfig-dir=.
kubectl dqlite bootstrap --id 1 --address ${API_SERVER_1_IP}:9000 --dir ${STORAGE_DIR}
```

Then copy the certificate files to the other control plane machines, e.g.:

```bash
scp ${CERT_DIR}/ca.crt kubelet:
scp ${CERT_DIR}/kubelet.conf kubelet:
for f in ca.crt ca.key sa.key sa.pub; do
  scp ${CERT_DIR}/${f} ${API_SERVER_2_IP}:${CERT_DIR}/
  scp ${CERT_DIR}/${f} ${API_SERVER_3_IP}:${CERT_DIR}/
done
```

And copy the kubelet configuration to the kubelet machine:

```
scp kubelet.conf <IP of machine 4>:
```

## Configuration of control plane specific to machine 2 and 3

This configure dqlite on the followers:

For machine 2 run:

```bash
kubectl dqlite join --id 2 --address ${API_SERVER_2_IP}:9000 --dir ${STORAGE_DIR} --cluster ${API_SERVER_1_IP}:9000
```

For machine 3 run:

```bash
kubectl dqlite join --id 3 --address ${API_SERVER_3_IP}:9000 --dir ${STORAGE_DIR} --cluster ${API_SERVER_1_IP}:9000,${API_SERVER_2_IP}:9000
```

## Common k8s configuration on machine 1, 2 and 3

This creates certificate and configuration files for the ```kube-apiserver```,
```kube-scheduler``` and ```kube-controller-manager``` control plane components, as well
as the ```admin.conf``` config file for the ```kubectl``` administration tool.

Run:

```bash
kubeadm init phase certs apiserver --config=${KUBEADM_CONF}
kubeadm init phase certs apiserver-kubelet-client --config=${KUBEADM_CONF}
kubeadm init phase kubeconfig scheduler --config=${KUBEADM_CONF} --kubeconfig-dir=.
kubeadm init phase kubeconfig controller-manager --config=${KUBEADM_CONF} --kubeconfig-dir=.
kubeadm init phase kubeconfig admin --config=${KUBEADM_CONF} --kubeconfig-dir=.
```

## Bring up the cluster

These steps will bring the cluster online. Should any of them fail, you can
reset the state and start from scratch by making sure that all systemd services
on machines 1, 2, 3 and 4 are stopped and the related process have died, and
then running:

```bash
# For machines 1, 2 and 3
cd ~
rm -rf ./backend
# Then on machine 0:
kubectl dqlite bootstrap --id 1 --address ${API_SERVER_1_IP}:9000 --dir ${STORAGE_DIR}
# on machine 1:
kubectl dqlite join --id 2 --address ${API_SERVER_2_IP}:9000 --dir ${STORAGE_DIR} --cluster ${API_SERVER_1_IP}:9000
# on machine:
kubectl dqlite join --id 3 --address ${API_SERVER_3_IP}:9000 --dir ${STORAGE_DIR} --cluster ${API_SERVER_1_IP}:9000,${API_SERVER_2_IP}:9000
```

and

```bash
# For machine 4
# Kill any leftover docker process and unmount pod volumes
sudo rm -rf /var/lib/kubelet/*
```

This will bring you back to the same state you had when you got here.

### Start API server on machine 1

Begin with the ```kube-apiserver``` on machine 1:

```bash
sudo systemctl start kube-apiserver
```

Look at ```sudo systemctl status kube-apiserver``` and check that no error is
reported. After a few second you can run ```kubectl``` and it should work, e.g.:

```bash
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ kubectl --kubeconfig ./admin.conf get services
NAME         TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)   AGE
kubernetes   ClusterIP   10.0.0.1     <none>        443/TCP   20s
```

### Start API server on machine 2

On machine 2 run:

```
sudo systemctl start kube-apiserver
```

Look at ```sudo systemctl status kube-apiserver``` and check that no error is
reported. After a few second the new node should have joined the cluster.

From **machine 0** you should be able to see the new node, e.g.:

```bash
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ echo "select * from servers" | sqlite3 /home/ubuntu/backend/servers.sql
10.55.60.113:9000
10.55.60.84:9000
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ kubectl-dqlite --kubeconfig ./admin.conf list
ID    Address
1     10.55.60.113:9000
2     10.55.60.84:9000
```

### Start API server on machine 3

Same steps as for machine 2.

### Start scheduler and controller on machine 1

On machine 1 run:

```
sudo systemctl start kube-scheduler
sudo systemctl start kube-controller-manager
```

Check that no error occured. After a few seconds you should see that both of
them have successfully acquired leadership, e.g:

```bash
kubectl --kubeconfig ./admin.conf describe endpoints -n kube-system

Name:         kube-controller-manager
Namespace:    kube-system
Labels:       <none>
Annotations:  control-plane.alpha.kubernetes.io/leader:
                {"holderIdentity":"master-62b68a8d-12d1-4507-b212-63212e29b975_b6614eaf-ed41-433e-b19e-e35e9028029b","leaseDurationSeconds":20,"acquireTim...
Subsets:
Events:
  Type    Reason          Age   From                     Message
  ----    ------          ----  ----                     -------
  Normal  LeaderElection  3s    kube-controller-manager  master-62b68a8d-12d1-4507-b212-63212e29b975_b6614eaf-ed41-433e-b19e-e35e9028029b became leader


Name:         kube-scheduler
Namespace:    kube-system
Labels:       <none>
Annotations:  control-plane.alpha.kubernetes.io/leader:
                {"holderIdentity":"master-62b68a8d-12d1-4507-b212-63212e29b975_668ab7d4-6be7-45cd-bb2c-6bdc456f5214","leaseDurationSeconds":20,"acquireTim...
Subsets:
Events:
  Type    Reason          Age   From               Message
  ----    ------          ----  ----               -------
  Normal  LeaderElection  9s    default-scheduler  master-62b68a8d-12d1-4507-b212-63212e29b975_668ab7d4-6be7-45cd-bb2c-6bdc456f5214 became leader
```

### Start scheduler and controller on machine 2 and 3.

Run:

```
sudo systemctl start kube-scheduler
sudo systemctl start kube-controller-manager
```

on both machine 2 and 3 and check that no error occured.

Check that running ```kubectl --kubeconfig ./admin.conf describe endpoints -n
kube-system``` still reports the same leaders.

### Start the kubelet on machine 4

Run:

```
sudo apt-get install docker.io
sudo mkdir /etc/kubernetes
sudo mkdir /var/lib/kubelet
sudo mkdir -p /var/run/kubernetes/static-pods

HOSTNAME_OVERRIDE=$(cat kubelet.conf |grep "\- name: system:node"|cut -f 4 -d :)

sudo -E cat | sudo tee /etc/kubernetes/config > /dev/null <<EOF
LOG_LEVEL=3
CERT_DIR=/home/ubuntu/certs
EOF

sudo -E cat | sudo tee /etc/kubernetes/kubelet > /dev/null <<EOF
HOSTNAME_OVERRIDE=${HOSTNAME_OVERRIDE}
EOF

sudo -E cat | sudo tee /etc/systemd/system/kubelet.service > /dev/null <<EOF
[Unit]
Description=Kubernetes Kubelet Server
After=docker.service
Requires=docker.service

[Service]
WorkingDirectory=/var/lib/kubelet
EnvironmentFile=-/etc/kubernetes/config
EnvironmentFile=-/etc/kubernetes/kubelet
Environment="LD_LIBRARY_PATH=/home/ubuntu/go/deps/sqlite/.libs/:/home/ubuntu/go/deps/libco/:/home/ubuntu/go/deps/raft/.libs/:/home/ubuntu/go/deps/dqlite/.libs/"
ExecStart=/usr/bin/hyperkube kubelet \
         --v=\${LOG_LEVEL} \
         --kubeconfig=/home/ubuntu/kubelet.conf \
         --client-ca-file=/home/ubuntu/ca.crt \
         --fail-swap-on=false \
         --vmodule="" \
         --chaos-chance=0.0 \
         --container-runtime=docker \
         --cloud-provider="" \
         --cloud-config="" \
         --feature-gates=AllAlpha=false \
         --cpu-cfs-quota=true \
         --enable-controller-attach-detach=true \
         --cgroups-per-qos=true \
         --cgroup-driver=cgroupfs \
         --cgroup-root="" \
         --pod-manifest-path=/var/run/kubernetes/static-pods \
         --authorization-mode=Webhook \
         --authentication-token-webhook \
         --hostname-override=\${HOSTNAME_OVERRIDE} \
         --cluster-dns=10.0.0.10 \
         --cluster-domain=cluster.local \
         --runtime-request-timeout=2m
Restart=on-failure
KillMode=process

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl start kubelet
```

Check that no error occured. After a few second the node should register itself
and become ready, e.g from machine 0:

```bash
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ kubectl --kubeconfig ./admin.conf get nodes
NAME                                          STATUS   ROLES    AGE   VERSION
master-62b68a8d-12d1-4507-b212-63212e29b975   Ready    <none>   11s   v1.16.0-alpha.0.2215+b5bc97a58d00da
```

### Deploy a sample workload

From machine 0 run:

```
kubectl --kubeconfig ./admin.conf create deployment hello-node --image=gcr.io/hello-minikube-zero-install/hello-node
```

And check that the pod is eventually started:

```bash
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ kubectl --kubeconfig ./admin.conf get pods
NAME                            READY   STATUS    RESTARTS   AGE
hello-node-8c45b9cf8-n6s5r      1/1     Running   0          119s
```

If any error occur, go back to "Bring up the cluster".

## Run the demo

From machine 0 run:

```
sudo systemctl stop kube-apiserver
```

Wait a few seconds for the leadership change to occur and check that the control
plane is still responsive:

```
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ kubectl --kubeconfig ./admin.conf get services
NAME         TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)   AGE
kubernetes   ClusterIP   10.0.0.1     <none>        443/TCP   20s
```

and that no leadership change occurred for the scheduler and controller-manager
components (```kubectl --kubeconfig ./admin.conf describe endpoints -n kube-system```).

Now stop the scheduler and the controller manager on machine 0:

```bash
sudo systemctl stop kube-scheduler
sudo systemctl stop kube-controller-manager
```

and observe that a leadership change eventually occurs (that should take between
~20 and ~25 seconds):

```bash
ubuntu@master-62b68a8d-12d1-4507-b212-63212e29b975:~$ kubectl --kubeconfig ./admin.conf describe endpoints -n kube-system
Name:         kube-controller-manager
Namespace:    kube-system
Labels:       <none>
Annotations:  control-plane.alpha.kubernetes.io/leader:
                {"holderIdentity":"master-fab4b1ba-0a50-41e9-af59-821820cfb87f_476b7226-228e-4aac-857d-ae215da57762","leaseDurationSeconds":20,"acquireTim...
Subsets:
Events:
  Type    Reason          Age   From                     Message
  ----    ------          ----  ----                     -------
  Normal  LeaderElection  104m  kube-controller-manager  master-62b68a8d-12d1-4507-b212-63212e29b975_b6614eaf-ed41-433e-b19e-e35e9028029b became leader
  Normal  LeaderElection  1s    kube-controller-manager  master-fab4b1ba-0a50-41e9-af59-821820cfb87f_476b7226-228e-4aac-857d-ae215da57762 became leader


Name:         kube-scheduler
Namespace:    kube-system
Labels:       <none>
Annotations:  control-plane.alpha.kubernetes.io/leader:
                {"holderIdentity":"master-a68283bd-5fbc-4717-8ead-c846d5b7b145_074af2b3-8c25-43de-b19b-66d9abd5b863","leaseDurationSeconds":20,"acquireTim...
Subsets:
Events:
  Type    Reason          Age   From               Message
  ----    ------          ----  ----               -------
  Normal  LeaderElection  104m  default-scheduler  master-62b68a8d-12d1-4507-b212-63212e29b975_668ab7d4-6be7-45cd-bb2c-6bdc456f5214 became leader
  Normal  LeaderElection  9s    default-scheduler  master-a68283bd-5fbc-4717-8ead-c846d5b7b145_074af2b3-8c25-43de-b19b-66d9abd5b863 became leader
```

Finally, delete the deployment and observe that pods get terminated and
eventually disappear (```kubectl --kubeconfig ./admin.conf get pods```):

```bash
kubectl --kubeconfig ./admin.conf delete deployment hello-node
```
