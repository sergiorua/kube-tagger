# Kubernetes Volume Tagger

## What?

It's a simple pod that checks if AWS EBS volumes created by K8s have the AWS tags required.

## How?

On your volume claims add the tags into annotations like:

```yaml
annotations:
  volume.beta.kubernetes.io/additional-resource-tags: Owner=Sergio,Environment=Dev
```

Multiple tags are `,` separated by default but you can override it with:

```yaml
annotations:
  volume.beta.kubernetes.io/additional-resource-tags-separator: ";"
```


You may need to grant your EC2 instances permissions to tag volumes. This is the minimal config expected:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ec2:DescribeVolumes",
      "Resource": "*"
     },
     {
       "Effect": "Allow",
       "Action": [
         "ec2:CreateTags"
       ],
       "Resource": "arn:aws:ec2:*:*:volume/*",
       "Condition": {
         "StringEquals": {
             "ec2:CreateAction" : "CreateTags"
        }
      }
    }
  ]
}
```
## Deploy

### Manual

See [kube-tagger.yaml](https://github.com/sergiorua/kube-tagger/blob/master/kube-tagger.yaml) for an example deployment.

```sh
kubectl apply -f https://raw.githubusercontent.com/sergiorua/kube-tagger/master/kube-tagger.yaml
```

### Helm

```sh
helm repo add kube-tagger https://sergiorua.github.io/kube-tagger
helm upgrade --install kube-tagger kube-tagger/kube-tagger
```

#### Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| extraEnv | object | `{}` |  |
| global.name | string | `"kube-tagger"` |  |
| global.namespace | string | `"kube-system"` |  |
| image.backoffLimit | int | `3` |  |
| image.containerPort | int | `8080` |  |
| image.namespace | string | `"kube-system"` |  |
| image.pullPolicy | string | `"Always"` |  |
| image.repository | string | `"sergrua/kube-tagger"` |  |
| image.tag | string | `"release-0.0.9"` |  |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| rbac.create | bool | `true` |  |
| rbac.pspEnabled | bool | `false` |  |
| replicas | int | `1` |  |
| resources | object | `{}` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.create | bool | `true` |  |
| tolerations | list | `[]` |  |
