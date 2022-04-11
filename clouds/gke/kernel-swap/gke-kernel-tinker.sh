#!/bin/bash
set -ex
region=us-central1-a
name="${USER}-tinker"
bootstrap=${BOOTSTRAP:-false}
cleanup=${CLEAN:-true}
num=${NODES:-2}
itype=${TYPE:-e2-medium}

if $cleanup; then
    cilium uninstall
fi

if $bootstrap; then
    gcloud container clusters create "$name" \
        --labels "usage=tinker,owner=${USER}" \
        --num-nodes $num \
        --machine-type $itype \
        --node-taints node.cilium.io/agent-not-ready=true:NoSchedule \
        --image-type=UBUNTU_CONTAINERD \
        --region $region 
    #cilium install
    cilium install --version -service-mesh:v1.11.0-beta.1 --config enable-envoy-config=true --kube-proxy-replacement=probe

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
# Install usual tooling
if $bootstrap; then
  helm install prometheus prometheus-community/prometheus --namespace prometheus  --namespace prometheus --create-namespace
fi
