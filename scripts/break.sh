#!/usr/bin/env bash
# Kubernetes troubleshooting drills: deploys a broken copy of the wiki chart as Helm release "lab"
# into namespace "lab" (outside ArgoCD, so selfHeal does not undo the breakage).
#   scripts/break.sh list | <scenario> | fix
set -euo pipefail
cd "$(dirname "$0")/.."
tag=$(awk '/^  tag:/{print $2}' charts/wiki/values.yaml)   # the image ArgoCD runs, so it exists in GHCR

# The tag goes in as a values file before the scenario: a later -f wins, while --set would override
# every file, including 1-bad-image.yaml.
deploy() {
  helm upgrade --install lab charts/wiki -n lab --create-namespace \
    -f scenarios/_lab.yaml -f <(printf 'image:\n  tag: %s\n' "$tag") "$@" >/dev/null
}

case ${1:-list} in
  list)
    ls scenarios | sed -n 's/^\([0-9].*\)\.yaml$/\1/p'
    echo 8-dns
    ;;
  fix)
    kubectl -n kube-system scale deploy coredns --replicas=2 >/dev/null
    deploy
    echo "lab is back to the working values"
    ;;
  8-dns)
    deploy
    kubectl -n kube-system scale deploy coredns --replicas=0 >/dev/null
    ;;
  *)
    [ -f "scenarios/$1.yaml" ] || { echo "unknown scenario: $1 (see: $0 list)"; exit 1; }
    deploy -f "scenarios/$1.yaml"
    ;;
esac

if [[ ${1:-list} != list && $1 != fix ]]; then
  cat <<MSG
Broken: $1. Incident: "the wiki in namespace lab does not answer". Find the cause and explain it out loud.
  kubectl -n lab get pods,svc,endpointslices
  kubectl -n lab run curl --rm -it --restart=Never --image=curlimages/curl:8.11.1 -- curl -sS -m 3 http://lab-wiki
Answers: README.md → "Troubleshooting drills". Reset: $0 fix
MSG
fi
