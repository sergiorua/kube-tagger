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
      "Sid": "AllowCreateTaggedVolumes",
      "Effect": "Allow",
      "Action": "ec2:CreateTags",
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

See [kube-tagger.yaml](https://github.com/sergiorua/kube-tagger/blob/master/kube-tagger.yaml) for an example deployment.

```sh
kubectl apply -f https://raw.githubusercontent.com/sergiorua/kube-tagger/master/kube-tagger.yaml
```
