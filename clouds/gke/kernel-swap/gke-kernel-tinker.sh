#!/bin/bash
set -ex
region=us-central1-a
name="${USER}-tinker"
gcloud container clusters create "$name" \
    --labels "usage=tinker,owner=${USER}" \
    --num-nodes 2 \
    --node-taints node.cilium.io/agent-not-ready=true:NoSchedule \
    --image-type=UBUNTU_CONTAINERD \
    --region $region 
cilium install
kubectl create -f cm-kernel.yaml
envsubst < ds-kernel.yaml
export KERNEL_VERSION=${KERNEL:-v5.4}
envsubst < ds-kernel.yaml | kubectl apply -f -
kubectl rollout status ds/node-initializer
for node in $(kubectl get nodes -o=custom-columns=:.metadata.name | grep -v -e '^[[:space:]]*$'); do
    gcloud compute instances reset --zone $region $node
done
sleep 10 # Wait for k8s to catchup to the reboot 
kubectl wait --for=condition=Ready nodes --all --timeout=600s 
