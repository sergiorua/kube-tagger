# Kubernetes Volume Tagger

## What?

It's a simple pod that checks if volumes have the AWS tags required added.

## How?

On your volume claims annotations add something like:

```yaml
annotations:
  volume.beta.kubernetes.io/additional-resource-tags: Owner=Sergio;Environment=Dev
```

Multiple tags are ; separated.

## Deploy

See [kube-tagger.yaml kube-tagger.yaml] for an example deployment.

```sh
kubectl apply -f https://raw.githubusercontent.com/sergiorua/kube-tagger/master/kube-tagger.yaml
```
