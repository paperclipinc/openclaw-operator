# Deployment Guide

This guide covers deploying the OpenClaw Operator on various Kubernetes platforms.

## Prerequisites (All Platforms)

- Kubernetes >= 1.28
- `kubectl` configured with cluster admin access
- Helm >= 3.12 (if using Helm installation)
- `kustomize` >= 5.0 (if using Kustomize installation)

Optional:
- cert-manager (required only if enabling the validating/defaulting webhook)
- Prometheus Operator (required only if enabling ServiceMonitor resources)
- An Ingress controller (required only if enabling Ingress resources)

---

## Local Development (Kind)

[Kind](https://kind.sigs.k8s.io/) is the recommended way to develop and test the operator locally.

### 1. Create a Kind Cluster

```bash
kind create cluster --name openclaw-dev
```

### 2. Build and Load the Operator Image

```bash
# Build the image
make docker-build IMG=openclaw-operator:dev

# Load into Kind
kind load docker-image openclaw-operator:dev --name openclaw-dev
```

### 3. Install CRDs

```bash
make install
```

### 4. Deploy the Operator

Using Kustomize:

```bash
make deploy IMG=openclaw-operator:dev
```

Or using Helm:

```bash
helm install openclaw-operator charts/openclaw-operator \
  --namespace openclaw-system --create-namespace \
  --set image.repository=openclaw-operator \
  --set image.tag=dev \
  --set image.pullPolicy=Never
```

### 5. Create a Test Instance

```bash
kubectl create namespace openclaw

kubectl create secret generic openclaw-api-keys \
  --namespace openclaw \
  --from-literal=ANTHROPIC_API_KEY=sk-your-key

kubectl apply -f - <<EOF
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: dev-assistant
  namespace: openclaw
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
  storage:
    persistence:
      enabled: false
EOF
```

### 6. Verify

```bash
kubectl get openclawinstance -n openclaw
kubectl get pods -n openclaw
```

### 7. Verify namespaced RBAC on Kind (optional)

Use this when you want the operator **Role** in a workload namespace plus the small **clusterdefaults** **ClusterRole**, with `OPENCLAW_WATCH_NAMESPACES` set automatically (`rbac.namespaced=true`).

When the manager cache is restricted to specific namespaces, **`OpenClawSelfConfig` objects must live in one of those namespaces** (the same informer cache applies to all controllers).

1. Build and load the image. With **Podman**, `kind load docker-image` often fails; save a tarball and use `kind load image-archive` instead:

```bash
export KIND_CLUSTER_NAME=openclaw-dev   # same name as kind create cluster --name
podman build -t localhost/openclaw-operator:dev .
podman save localhost/openclaw-operator:dev -o /tmp/openclaw-operator-dev.tar
kind load image-archive /tmp/openclaw-operator-dev.tar --name "${KIND_CLUSTER_NAME}"
rm -f /tmp/openclaw-operator-dev.tar
```

2. Install Helm with namespaced RBAC (operator in `openclaw-system`, instances in `openclaw`). If CRDs are **already** on the cluster from a prior install, set `crds.install=false` once:

```bash
kubectl create namespace openclaw-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace openclaw --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install openclaw-operator charts/openclaw-operator -n openclaw-system \
  --set image.repository=localhost/openclaw-operator \
  --set image.tag=dev \
  --set image.pullPolicy=Never \
  --set rbac.namespaced=true \
  --set-json 'rbac.watchNamespaces=["openclaw"]' \
  --set networkPolicy.enabled=false \
  --set crds.install=false
```

3. Confirm env and RBAC:

```bash
kubectl get deploy -n openclaw-system -o jsonpath='{range .items[*].spec.template.spec.containers[0].env[*]}{.name}={.value}{"\n"}{end}' | grep OPENCLAW_WATCH_NAMESPACES
kubectl get role,rolebinding -n openclaw
kubectl get clusterrole | grep openclaw-operator | grep clusterdefaults
```

4. Scripted path: from the repo root, after `kubectl config use-context kind-<name>`:

```bash
export KIND_CLUSTER_NAME=<your-cluster-name>
./hack/kind-verify-namespaced-rbac.sh
```

### Cleanup

```bash
kind delete cluster --name openclaw-dev
```

---

## AWS EKS

### 1. Prerequisites

- AWS CLI configured with appropriate permissions
- `eksctl` installed
- An EKS cluster running Kubernetes >= 1.28

```bash
# Create a cluster (if needed)
eksctl create cluster \
  --name openclaw-cluster \
  --region us-east-1 \
  --version 1.30 \
  --nodegroup-name workers \
  --node-type m5.large \
  --nodes 3
```

### 2. Install the Amazon EBS CSI Driver

Required for persistent storage (PVCs). If not already installed:

```bash
eksctl create addon --name aws-ebs-csi-driver --cluster openclaw-cluster

# Create a gp3 StorageClass (recommended)
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
volumeBindingMode: WaitForFirstConsumer
parameters:
  type: gp3
  encrypted: "true"
EOF
```

### 3. Install the Operator

```bash
helm install openclaw-operator \
  oci://ghcr.io/openclaw-rocks/charts/openclaw-operator \
  --namespace openclaw-system --create-namespace \
  --set leaderElection.enabled=true
```

### 4. Create an Instance

```bash
kubectl create namespace openclaw

# Store API keys in a Secret
kubectl create secret generic openclaw-api-keys \
  --namespace openclaw \
  --from-literal=ANTHROPIC_API_KEY=sk-your-key

kubectl apply -f - <<EOF
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-assistant
  namespace: openclaw
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
  resources:
    requests:
      cpu: "1"
      memory: 2Gi
    limits:
      cpu: "2"
      memory: 4Gi
  storage:
    persistence:
      enabled: true
      storageClass: gp3
      size: 50Gi
  networking:
    service:
      type: ClusterIP
EOF
```

### 5. (Optional) Expose via AWS ALB

Install the AWS Load Balancer Controller and use Ingress:

```yaml
spec:
  networking:
    ingress:
      enabled: true
      className: alb
      annotations:
        alb.ingress.kubernetes.io/scheme: internet-facing
        alb.ingress.kubernetes.io/target-type: ip
        alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
        alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-east-1:123456789:certificate/abc
      hosts:
        - host: openclaw.example.com
          paths:
            - path: /
              pathType: Prefix
```

### 6. Verify

```bash
kubectl get openclawinstance -n openclaw
kubectl get pods -n openclaw
kubectl get svc -n openclaw
```

---

## Google GKE

### 1. Prerequisites

- `gcloud` CLI configured
- A GKE cluster running Kubernetes >= 1.28

```bash
# Create a cluster (if needed)
gcloud container clusters create openclaw-cluster \
  --zone us-central1-a \
  --num-nodes 3 \
  --machine-type e2-standard-4 \
  --enable-ip-alias

# Get credentials
gcloud container clusters get-credentials openclaw-cluster --zone us-central1-a
```

### 2. Storage

GKE provides a default `standard-rwo` StorageClass backed by Persistent Disks. No additional setup is required.

For SSD-backed storage:

```bash
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ssd
provisioner: pd.csi.storage.gke.io
volumeBindingMode: WaitForFirstConsumer
parameters:
  type: pd-ssd
EOF
```

### 3. Install the Operator

```bash
helm install openclaw-operator \
  oci://ghcr.io/openclaw-rocks/charts/openclaw-operator \
  --namespace openclaw-system --create-namespace \
  --set leaderElection.enabled=true
```

### 4. Create an Instance

```bash
kubectl create namespace openclaw

kubectl create secret generic openclaw-api-keys \
  --namespace openclaw \
  --from-literal=ANTHROPIC_API_KEY=sk-your-key

kubectl apply -f - <<EOF
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-assistant
  namespace: openclaw
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
  storage:
    persistence:
      enabled: true
      size: 50Gi
  networking:
    service:
      type: ClusterIP
EOF
```

### 5. (Optional) Expose via GKE Ingress

GKE includes a built-in Ingress controller. Alternatively, install nginx-ingress for WebSocket support:

```bash
helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace
```

Then configure the instance:

```yaml
spec:
  networking:
    ingress:
      enabled: true
      className: nginx
      hosts:
        - host: openclaw.example.com
          paths:
            - path: /
      tls:
        - hosts:
            - openclaw.example.com
          secretName: openclaw-tls
  security:
    networkPolicy:
      allowedIngressNamespaces:
        - ingress-nginx
```

### 6. Verify

```bash
kubectl get openclawinstance -n openclaw
kubectl get pods -n openclaw
```

---

## Azure AKS

### 1. Prerequisites

- Azure CLI (`az`) configured
- An AKS cluster running Kubernetes >= 1.28

```bash
# Create a resource group
az group create --name openclaw-rg --location eastus

# Create a cluster
az aks create \
  --resource-group openclaw-rg \
  --name openclaw-cluster \
  --node-count 3 \
  --node-vm-size Standard_D4s_v3 \
  --generate-ssh-keys

# Get credentials
az aks get-credentials --resource-group openclaw-rg --name openclaw-cluster
```

### 2. Storage

AKS provides `managed-csi` (Azure Disk) and `azurefile-csi` (Azure Files) StorageClasses by default.

For premium SSD storage:

```bash
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: managed-premium
provisioner: disk.csi.azure.com
volumeBindingMode: WaitForFirstConsumer
parameters:
  skuName: Premium_LRS
EOF
```

### 3. Install the Operator

```bash
helm install openclaw-operator \
  oci://ghcr.io/openclaw-rocks/charts/openclaw-operator \
  --namespace openclaw-system --create-namespace \
  --set leaderElection.enabled=true
```

### 4. Create an Instance

```bash
kubectl create namespace openclaw

kubectl create secret generic openclaw-api-keys \
  --namespace openclaw \
  --from-literal=ANTHROPIC_API_KEY=sk-your-key

kubectl apply -f - <<EOF
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-assistant
  namespace: openclaw
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
  storage:
    persistence:
      enabled: true
      storageClass: managed-csi
      size: 50Gi
  networking:
    service:
      type: ClusterIP
EOF
```

### 5. (Optional) Expose via Azure Application Gateway or nginx

Install nginx-ingress:

```bash
helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace \
  --set controller.service.annotations."service\.beta\.kubernetes\.io/azure-load-balancer-health-probe-request-path"=/healthz
```

Then configure the instance's Ingress as shown in the GKE section.

### 6. Verify

```bash
kubectl get openclawinstance -n openclaw
kubectl get pods -n openclaw
```

---

## Generic Kubernetes

This section covers installation on any conformant Kubernetes cluster (self-managed, on-premises, or other cloud providers).

### 1. Prerequisites

Verify your cluster is ready:

```bash
kubectl version
kubectl get nodes
kubectl get storageclass
```

### 2. Install via Helm

```bash
# Add the OCI registry (no separate repo add needed for OCI)
helm install openclaw-operator \
  oci://ghcr.io/openclaw-rocks/charts/openclaw-operator \
  --namespace openclaw-system --create-namespace \
  --set leaderElection.enabled=true
```

### 3. Install via Kustomize

```bash
# Clone the repository
git clone https://github.com/OpenClaw-rocks/openclaw-operator.git
cd openclaw-operator

# Install CRDs
make install

# Deploy the operator
make deploy IMG=ghcr.io/openclaw-rocks/openclaw-operator:v0.1.0
```

### 4. Create an Instance

```bash
kubectl create namespace openclaw

kubectl create secret generic openclaw-api-keys \
  --namespace openclaw \
  --from-literal=ANTHROPIC_API_KEY=sk-your-key

kubectl apply -f - <<EOF
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: my-assistant
  namespace: openclaw
spec:
  envFrom:
    - secretRef:
        name: openclaw-api-keys
EOF
```

### 5. Verify

```bash
# Check operator is running
kubectl get pods -n openclaw-system

# Check instance status
kubectl get openclawinstance -n openclaw

# Check managed resources
kubectl get all -n openclaw -l app.kubernetes.io/managed-by=openclaw-operator
```

---

## Post-Installation Verification

After deploying on any platform, run these checks to confirm everything is working:

```bash
# 1. Operator is running
kubectl get pods -n openclaw-system
# Expected: 1/1 Running

# 2. CRD is installed
kubectl get crd openclawinstances.openclaw.rocks
# Expected: CRD listed with creation date

# 3. Instance reaches Running phase
kubectl get openclawinstance -n openclaw
# Expected: Phase=Running, Ready=True

# 4. All managed resources exist
kubectl get deploy,svc,sa,role,rolebinding,networkpolicy,pdb -n openclaw \
  -l app.kubernetes.io/managed-by=openclaw-operator

# 5. Pod is healthy
kubectl get pods -n openclaw -l app.kubernetes.io/name=openclaw
# Expected: 1/1 Running (or 2/2 with Chromium sidecar)

# 6. Gateway is reachable (from within the cluster)
kubectl run -n openclaw test-curl --rm -it --image=curlimages/curl -- \
  curl -s -o /dev/null -w '%{http_code}' http://my-assistant:18789
```

## Uninstalling

```bash
# Delete all instances first (this triggers cleanup)
kubectl delete openclawinstance --all -n openclaw

# Wait for resources to be cleaned up
kubectl get all -n openclaw -l app.kubernetes.io/managed-by=openclaw-operator

# Uninstall the operator (Helm)
helm uninstall openclaw-operator -n openclaw-system

# CRDs are kept by default. To remove them:
kubectl delete crd openclawinstances.openclaw.rocks

# Remove namespaces
kubectl delete namespace openclaw openclaw-system
```
