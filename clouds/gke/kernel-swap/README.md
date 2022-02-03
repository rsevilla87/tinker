# kernel-swap for GKE

To define the kernel we should swap to, just 
```
export KERNEL=<version>
```
Then execute `./gke-kernel-tinker.sh` -- assumptions are that you have gcloud/gke setup already with creds :-)

## Caveat 

GKE has an `auto-repair` functionality, which will "repair" a node in the event the node is stuck in `NOT_READY` for > 10m.
To avoid having `auto-repair` troll you, delete all pods that you created (meaning, go back to a clean cluster state). Cleaning
will reduce the time it takes for all the pods to restart, and the node to get back into `READY`. Alternatively, see:

https://cloud.google.com/kubernetes-engine/docs/how-to/node-auto-repair
