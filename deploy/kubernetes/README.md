# Kubernetes Deployment

The standalone router and deterministic fake backend have Deployment and Service
resources for a two-backend Kubernetes topology. This W2 scope does not include a
controller, CRD, or Gateway resource; Gateway API Inference Extension resources
belong to the enhancement milestone.

The W2 fake topology is defined in `w2-fake.yaml` and uses locally built scratch
images with `imagePullPolicy: Never`. Run the complete temporary-cluster test with
`make e2e-kind`; the command deletes its uniquely named kind cluster on exit.
