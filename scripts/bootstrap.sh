#!/usr/bin/env bash
# Creates the kind cluster "lab", installs ArgoCD with Helm and hands everything else to GitOps (deploy/root.yaml).
set -euo pipefail
cd "$(dirname "$0")/.."
ARGOCD_CHART_VERSION=$(awk '/chart: argo-cd/{f=1} f&&/targetRevision/{print $2; exit}' deploy/apps/argocd.yaml)

kind get clusters | grep -qx lab || kind create cluster --config deploy/kind-config.yaml
kubectl config use-context kind-lab

helm repo add argo https://argoproj.github.io/argo-helm >/dev/null
helm repo update argo >/dev/null
helm upgrade --install argocd argo/argo-cd --version "$ARGOCD_CHART_VERSION" \
  -n argocd --create-namespace -f deploy/values/argocd.yaml --wait --timeout 10m

kubectl apply -f deploy/root.yaml

password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)
cat <<MSG

ArgoCD     http://localhost:8081   admin / $password
Grafana    http://localhost:3000   admin / admin   (dashboard "Wiki SLO")
Prometheus http://localhost:9090
Alertmanager http://localhost:9093
Wiki       http://localhost:8080

ArgoCD now syncs deploy/apps from GitHub: monitoring first, then the wiki (a few minutes).
  kubectl -n argocd get applications -w
MSG
