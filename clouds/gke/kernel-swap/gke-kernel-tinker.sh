#!/bin/bash
set -ex
region=us-central1-a
name="${USER}-tinker"
bootstrap=${BOOTSTRAP:-false}
cleanup=${CLEAN:-true}

if $cleanup; then
    cilium uninstall
fi

if $bootstrap; then
    gcloud container clusters create "$name" \
        --labels "usage=tinker,owner=${USER}" \
        --num-nodes 2 \
        --node-taints node.cilium.io/agent-not-ready=true:NoSchedule \
        --image-type=UBUNTU_CONTAINERD \
        --region $region 
    cilium install
    helm install prometheus prometheus-community/prometheus --namespace prometheus  --namespace prometheus --create-namespace
fi

kubectl apply -f cm-kernel.yaml
envsubst < ds-kernel.yaml
export KERNEL_VERSION=${KERNEL:-v5.4}
envsubst < ds-kernel.yaml | kubectl apply -f -
kubectl rollout status ds/node-initializer
for node in $(kubectl get nodes -o=custom-columns=:.metadata.name | grep -v -e '^[[:space:]]*$'); do
    gcloud compute instances reset --zone $region $node
done
kubectl delete -f ds-kernel.yaml
sleep 30 # Wait for k8s to catchup to the reboot 
kubectl wait --for=condition=Ready nodes --all --timeout=600s 
