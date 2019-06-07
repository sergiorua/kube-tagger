# Kubernetes Volume Tagger

## What?

It's a simple pod that checks if AWS EBS volumes created by K8s have the AWS tags required.

## How?

On your volume claims add the tags into annotations like:

```yaml
annotations:
  volume.beta.kubernetes.io/additional-resource-tags: Owner=Sergio;Environment=Dev
```

Multiple tags are `;` separated.

## Deploy

See [kube-tagger.yaml](https://github.com/sergiorua/kube-tagger/blob/master/kube-tagger.yaml) for an example deployment.

```sh
kubectl apply -f https://raw.githubusercontent.com/sergiorua/kube-tagger/master/kube-tagger.yaml
```
