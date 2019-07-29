# kube-tagger Helm Chart

This Helm Chart will create the kube-tagger Pod to create EBS tags on PVC

## Installation Commands

You can install the chart directly

```
helm upgrade --install kube-tagger ./kube-tagger/ --namespace=kube-system --debug --dry-run
helm upgrade --install kube-tagger ./kube-tagger/ --namespace=kube-system --debug 
```

If Helm-Chart is in a repo

```
helm upgrade --install kube-tagger chartmuseum/kube-tagger/ --namespace=kube-system --debug --dry-run
helm upgrade --install kube-tagger chartmuseum/kube-tagger/ --namespace=kube-system --debug 
```
