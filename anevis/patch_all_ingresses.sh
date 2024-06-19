#!/bin/bash

NAMESPACE="activemq-artemis-operator"

PATCH_FILE="anevis/ing-patch.yml"

INGRESSES=$(kubectl get ingress -n $NAMESPACE -o jsonpath='{.items[*].metadata.name}')

# Loop through each Ingress and apply the patch
for INGRESS in $INGRESSES; do
  echo "Patching ingress: $INGRESS"
  kubectl patch ingress $INGRESS -n $NAMESPACE --patch "$(cat $PATCH_FILE)"
done

