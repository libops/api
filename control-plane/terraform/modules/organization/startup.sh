#!/bin/bash

# Pull Customer Org VM Docker image
# Assuming we have a way to auth to GCR, or image is public
# For private GCR, we need to configure docker-credential-gcr
gcloud auth configure-docker gcr.io -q

docker pull gcr.io/libops/customer-vm:latest

# Run Customer Org VM service
docker run -d \
  --name customer-vm \
  --restart always \
  --network host \
  -e ORCHESTRATOR_URL=wss://${orchestrator_psc_ip}:8081/ws \
  -e ORGANIZATION_ID=${organization_id} \
  -e GCS_BUCKET=${gcs_bucket} \
  -v /var/lib/terraform:/terraform \
  -v /var/lib/vault:/vault \
  gcr.io/libops/customer-vm:latest
