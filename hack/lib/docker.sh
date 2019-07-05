#!/usr/bin/env bash

# A set of helpers for starting/running docker for tests

kube::docker::start() {
  DOCKER_BRIDGE=${DOCKER_BRIDGE:-"kubelet"}
  DOCKER_NET=${DOCKER_NET:-"10.1.30.1/24"}

  if ! ip link show ${DOCKER_BRIDGE} > /dev/null 2>&1; then
    sudo ip link add name ${DOCKER_BRIDGE} type bridge
    sudo ip addr add ${DOCKER_NET} dev ${DOCKER_BRIDGE}
    sudo ip link set dev ${DOCKER_BRIDGE} up
  fi
  sudo systemd-run --unit="docker-${DOCKER_BRIDGE}" --service-type=notify \
       dockerd \
       --bridge=${DOCKER_BRIDGE} \
       --data-root=${DOCKER_DIR} \
       --exec-root=${DOCKER_DIR} \
       --host=unix://${DOCKER_DIR}/socket \
       --containerd=/run/containerd/containerd.sock \
       --pidfile=${DOCKER_DIR}/pid
  DOCKER_PID="yes"

  echo "Waiting for docker to come up."
  kube::util::wait_for_socket ${DOCKER_DIR}/socket http://localhost/info "docker: " 0.25 80
}

kube::docker::stop() {
  if [[ -n "${DOCKER_PID-}" ]]; then
    sudo systemctl stop "docker-${DOCKER_BRIDGE}"
  fi
}
