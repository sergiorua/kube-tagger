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
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var verbose bool
var local bool
var kubeconfig string

func init() {
	flag.BoolVar(&verbose, "v", false, "Verbose")
	flag.BoolVar(&local, "l", false, "Run outside kube cluster (dev purposes)")

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
	// local = true
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
		log.Fatalln(err.Error())
	}

	watcher, err := clientset.CoreV1().PersistentVolumeClaims("").Watch(metav1.ListOptions{})
	if err != nil {
		log.Fatalln(err)
	}
	/* changes */
	ch := watcher.ResultChan()

	for event := range ch {
		pvc, ok := event.Object.(*v1.PersistentVolumeClaim)
		if !ok {
			log.Fatal("unexpected type")
		}
		if event.Type == watch.Added || event.Type == watch.Modified {
			namespace := pvc.GetNamespace()
			volumeClaimName := pvc.GetName()
			volumeClaim := *pvc

			if isEBSVolume(&volumeClaim) {
				volumeName := volumeClaim.Spec.VolumeName
				awsVolume, errp := clientset.CoreV1().PersistentVolumes().Get(volumeName, metav1.GetOptions{})
				if errp != nil {
					log.Printf("Cannot find EBS volume associated with %s: %s", volumeName, errp)
					continue
				}

				awsVolumeID := awsVolume.Spec.PersistentVolumeSource.AWSElasticBlockStore.VolumeID

				log.Printf("Volume Claim: %s\n", volumeClaimName)
				log.Printf("\tEvent Type: %s", event.Type)
				log.Printf("\tNamespace: %s\n", namespace)
				log.Printf("\tVolume: %s\n", volumeName)
				log.Printf("\tAWS Volume ID: %s\n", awsVolumeID)

				separator := ","
				tagsToAdd := ""
				for k, v := range volumeClaim.Annotations {
					if k == "volume.beta.kubernetes.io/additional-resource-tags-separator" {
						separator = v
					}

					if k == "volume.beta.kubernetes.io/additional-resource-tags" {
						tagsToAdd = v
					}
				}
				if tagsToAdd != "" {
					addAWSTags(tagsToAdd, awsVolumeID, separator)
				}
			} else {
				log.Printf("\t=> Volume is not EBS. Ignoring")
			}
		}
	}
}

/*
	This only works for EBS volumes. Make sure they are!
	TODO: remove kuberentes.io/ebs.csi.aws.com after test
*/
func isEBSVolume(volume *v1.PersistentVolumeClaim) bool {
	for k, v := range volume.Annotations {
		if k == "volume.beta.kubernetes.io/storage-provisioner" && (v == "kubernetes.io/aws-ebs" || v == "kubernetes.io/ebs.csi.aws.com" || v == "ebs.csi.aws.com") {
			return true
		}
	}
	return false
}

/*
	Loops through the tags found for the volume and calls `setTag`
	to add it via the AWS api
*/
func addAWSTags(awsTags string, awsVolumeID string, separator string) {
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
		log.Printf("Cannot get volume %s", awsVolume)
		log.Println(err)
		return
	}
	tags := strings.Split(awsTags, separator)
	for i := range tags {
		log.Printf("\tAdding tag %s to EBS Volume %s\n", tags[i], awsVolume)
		t := strings.Split(tags[i], "=")
		if len(t) != 2 {
			log.Printf("Malformed tag found, skipping: %v", t)
			continue
		}
		if !hasTag(resp.Volumes[0].Tags, t[0], t[1]) {
			setTag(svc, t[0], t[1], awsVolume)
		}
	}
}

/*
	AWS api call to set the tag found in the annotations
*/
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
		log.Fatal(err)
		return false
	}
	if verbose {
		log.Println(ret)
	}
	return true
}

/*
   Check if the tag is already set. It wouldn't be a problem if it is
   but if you're using cloudtrail it may be an issue seeing it
   being set all muiltiple times
*/
func hasTag(tags []*ec2.Tag, Key string, value string) bool {
	for i := range tags {
		if *tags[i].Key == Key && *tags[i].Value == value {
			log.Printf("\t\tTag %s already set with value %s\n",
				*tags[i].Key,
				*tags[i].Value)
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

func stringInSlice(str string, list []string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
