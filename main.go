/*
Copyright 2018 Sergio Rua

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	v1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func isEBSVolume(volume *v1.PersistentVolumeClaim) (bool) {
	for k,v := range volume.Annotations {
		if k == "volume.beta.kubernetes.io/storage-provisioner" && v == "kubernetes.io/aws-ebs" {
			return true
		}
	}
	return false
}

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	volumeClaims, err := clientset.CoreV1().PersistentVolumeClaims("").List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for i := range volumeClaims.Items {
		namespace := volumeClaims.Items[i].GetNamespace()
		volumeClaimName := volumeClaims.Items[i].GetName()
		volumeClaim, errp := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(volumeClaimName, metav1.GetOptions{})
		if errp != nil {
			fmt.Println(errp)
			continue
		}
		volumeName := volumeClaim.Spec.VolumeName

		awsVolume, errp := clientset.CoreV1().PersistentVolumes().Get(volumeName, metav1.GetOptions{})
		if errp != nil {
			fmt.Printf("Cannot find EBS volme associated with %s: %s", volumeName, errp)
			continue
		}
		awsVolumeId := awsVolume.Spec.PersistentVolumeSource.AWSElasticBlockStore.VolumeID

		fmt.Printf("\nVolume Claim: %s\n", volumeClaimName)
		fmt.Printf("\tVolume: %s\n", volumeName)
		fmt.Printf("\tAWS Volume ID: %s\n", awsVolumeId)
		if isEBSVolume(volumeClaim) {
			for k,v := range volumeClaim.Annotations {
				if k == "volume.beta.kubernetes.io/additional-resource-tags" {
					//fmt.Printf("\tAdding tags found on %s: %s\n", k, v)
					addAWSTags(v, awsVolumeId)
				}
			}
		}
	}
}

func addAWSTags(awsTags string, awsVolumeID string) {
	awsRegion, awsVolume := splitVol(awsVolumeID)

	/* Connect to AWS */
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		panic(err)
	}

	svc := ec2.New(sess)

	params := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{&awsVolume},
	}

	resp, err := svc.DescribeVolumes(params)
	if err != nil {
		fmt.Printf("Cannot get volume %s", awsVolume)
		fmt.Println(err)
		return;
	}
	fmt.Println(resp)

	tags := strings.Split(awsTags, ";")
	for i := range tags {
		fmt.Printf("\tAdding tag %s to EBS Volume %s\n", tags[i], awsVolume)
		t := strings.Split(tags[i], "=")
		setTag(svc, t[0], t[1], awsVolume)
	}
}

func setTag(svc *ec2.EC2, tagKey string, tagValue string, volumeID string) bool {
    tags := &ec2.CreateTagsInput{
		Resources: []*string{
			aws.String(volumeID),
		},
        Tags: []*ec2.Tag{
            {
                Key:   aws.String(tagKey),
                Value: aws.String(tagValue),
            },
        },
    }
    ret, err := svc.CreateTags(tags)
    if err != nil {
        fmt.Println(err)
        return false
	}
	fmt.Println(ret)
    return true
}


/* Take a URL as returned by Kubernetes in the format

	aws://eu-west-1b/vol-7iyw8ygidg

	and returns region and volume name
*/
func splitVol(vol string) (string, string) {
	sp := strings.Split(vol, "/")
	re := regexp.MustCompile(`[a-z]$`)
	zone := re.ReplaceAllString(sp[2], "")

	return zone, sp[3]
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
