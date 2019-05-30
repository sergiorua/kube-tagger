/*
Copyright 2019 Sergio Rua

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var verbose bool
var local bool
var kubeconfig string
var oneshot bool

func init() {
	flag.BoolVar(&verbose, "d", false, "Verbose")
	flag.BoolVar(&local, "l", false, "Run outside kube cluster")
	flag.BoolVar(&oneshot, "1", false, "Run only once and exit")

	if home := homeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
}


func main() {
	flag.Parse()
	var config *rest.Config
	var err error

	if local == false {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
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

	for {
		for i := range volumeClaims.Items {
			namespace := volumeClaims.Items[i].GetNamespace()
			volumeClaimName := volumeClaims.Items[i].GetName()
			volumeClaim := volumeClaims.Items[i]
			volumeName := volumeClaim.Spec.VolumeName

			awsVolume, errp := clientset.CoreV1().PersistentVolumes().Get(volumeName, metav1.GetOptions{})
			if errp != nil {
				fmt.Printf("Cannot find EBS volme associated with %s: %s", volumeName, errp)
				continue
			}
			awsVolumeID := awsVolume.Spec.PersistentVolumeSource.AWSElasticBlockStore.VolumeID

			fmt.Printf("\nVolume Claim: %s\n", volumeClaimName)
			fmt.Printf("\tNamespace: %s\n", namespace)
			fmt.Printf("\tVolume: %s\n", volumeName)
			fmt.Printf("\tAWS Volume ID: %s\n", awsVolumeID)
			if isEBSVolume(&volumeClaim) {
				for k,v := range volumeClaim.Annotations {
					if k == "volume.beta.kubernetes.io/additional-resource-tags" {
						addAWSTags(v, awsVolumeID)
					}
				}
			}
		}

		if oneshot {
			break
		}
		// Sleep 10 minutes
		time.Sleep(600 * time.Millisecond)
	}
}


func isEBSVolume(volume *v1.PersistentVolumeClaim) (bool) {
	for k,v := range volume.Annotations {
		if k == "volume.beta.kubernetes.io/storage-provisioner" && v == "kubernetes.io/aws-ebs" {
			return true
		}
	}
	return false
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
	tags := strings.Split(awsTags, ";")
	for i := range tags {
		fmt.Printf("\tAdding tag %s to EBS Volume %s\n", tags[i], awsVolume)
		t := strings.Split(tags[i], "=")
		if !hasTag(resp.Volumes[0].Tags, t[0], t[1]) {
			setTag(svc, t[0], t[1], awsVolume)
		}
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
	if verbose {
		fmt.Println(ret)
	}
    return true
}

func hasTag(tags []*ec2.Tag, Key string, value string) (bool) {
	for i := range tags {
		if *tags[i].Key == Key && *tags[i].Value == value {
			fmt.Printf("\t\tTag %s already set with value %s\n", *tags[i].Key, *tags[i].Value)
			return true
		}
	}
	return false
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
