# ActiveMQ Artemis Operator

This project is a [Kubernetes](https://kubernetes.io/) [operator](https://coreos.com/blog/introducing-operators.html)
to manage the [Apache ActiveMQ Artemis](https://activemq.apache.org/artemis/) message broker.

## Status ##

The current api version of all main CRDs managed by the operator is **v1beta1**.

## Quickstart

The [quickstart.md](docs/getting-started/quick-start.md) provides simple steps to quickly get the operator up and running
as well as deploy/managing broker deployments.

## Building

The [building.md](docs/help/building.md) describes how to build operator and how to test your changes

## OLM integration

The [bundle.md](docs/help/bundle.md) contains instructions for how to build operator bundle images and integrate it into [Operator Liftcycle Manager](https://olm.operatorframework.io/) framework.

## Debugging operator inside a container

Install delve in the `builder` container, i.e. `RUN go install github.com/go-delve/delve/cmd/dlv@latest`
Disable build optimization, i.e. `go build -gcflags="all=-N -l"`
Copy delve to the `base-env` container, i.e. `COPY --from=builder /go/bin/dlv /bin`
Execute operator using delve, i.e. `/bin/dlv exec --listen=0.0.0.0:40000 --headless=true --api-version=2 --accept-multiclient ${OPERATOR} $@`

# Anevis Activemq-Operator questions
## Updating the cluster
### General Update procedure
See operator/eks-work.md
### Running updates while messages are in the cluster?

## Cluster Load Test
Test the cluster under sustained load (evtl. with a Test Pc). 
Currently, the cluster has no problem processing several thousand messages. (3k)

## Clustering Questions
### Changes in the broker.xml when using deploymentPlan.size > 1

## Stopping some pods 
### What happens with the messages from a single pod when the pod is stopped. Are they redistributed? 
See [this guide](https://developers.redhat.com/articles/2023/12/05/how-use-message-migration-amq-broker-operator#).
It covers this exact issue. Scaling down the deployment. 

Verified with:
```shell
kubectl exec artemis-address-queue-ss-0 -n myproject -- /bin/bash /home/jboss/amq-broker/bin/artemis queue stat --user admin --password admin --url tcp://artemis-address-queue-ss-0:61616
```
* I don't know why this works, even with the wrong credentials. 
* It was also possible after some time to access the eks webconsole (after scaling and redeploying)

THIS WORKS! 
I scaled it to 0 (with the help of the artemis_cluster_persistence.yaml file). After scaling back again, 
the messages still persisted. 

### How difficult is it to merge the EBS storage from on pod with the storage of another pod.