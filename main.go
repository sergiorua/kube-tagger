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
	"os"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"gopkg.in/alecthomas/kingpin.v2"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	debug      = kingpin.Flag("debug", "Enable debug logging").Bool()
	local      = kingpin.Flag("local", "Run locally for development").Bool()
	kubeconfig = kingpin.Flag("kubeconfig", "Path to kubeconfig").OverrideDefaultFromEnvar("KUBECONFIG").String()
)

func init() {
	kingpin.Version("0.0.1")
	kingpin.Parse()
}

func main() {
	var config *rest.Config
	var err error

	if !*local {
		config, err = rest.InClusterConfig()
		if err != nil {
			log.WithError(err).Fatal("Error building In Cluster Config")
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			log.WithError(err).Fatalf("Error creating config from kubeconfig: %s", *kubeconfig)
		}
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithError(err).Fatal("Error creating clientset")
	}

	watcher, err := clientset.CoreV1().PersistentVolumeClaims("").Watch(metav1.ListOptions{})
	if err != nil {
		log.WithError(err).Fatal("Error creating PVC watcher")
	}
	/* changes */
	ch := watcher.ResultChan()

	for event := range ch {
		pvc, ok := event.Object.(*v1.PersistentVolumeClaim)
		if !ok {
			log.Fatal("unexpected event type")
		}
		if event.Type == watch.Added || event.Type == watch.Modified {
			namespace := pvc.GetNamespace()
			volumeClaimName := pvc.GetName()
			volumeClaim := *pvc
			volumeName := volumeClaim.Spec.VolumeName

			eventLogger := log.WithFields(log.Fields{
				"namespace":       namespace,
				"volumeClaimName": volumeClaimName,
				"volumeName":      volumeName,
			})

			awsVolume, errp := clientset.CoreV1().PersistentVolumes().Get(volumeName, metav1.GetOptions{})
			if errp != nil {
				eventLogger.WithError(errp).Error("Cannot find EBS volume associated with Volume Claim")
				continue
			}
			awsVolumeID := awsVolume.Spec.PersistentVolumeSource.AWSElasticBlockStore.VolumeID

			eventLogger.WithFields(log.Fields{"awsVolumeID": awsVolumeID, "eventType": event.Type}).Info("Processing Volume Tags")
			if isEBSVolume(&volumeClaim) {
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
					addAWSTags(tagsToAdd, awsVolumeID, separator, *eventLogger)
				}
			} else {
				eventLogger.Info("Volume is not EBS. Ignoring")
			}
		}
	}
}

/*
	This only works for EBS volumes. Make sure they are!
*/
func isEBSVolume(volume *v1.PersistentVolumeClaim) bool {
	for k, v := range volume.Annotations {
		if k == "volume.beta.kubernetes.io/storage-provisioner" && v == "kubernetes.io/aws-ebs" {
			return true
		}
	}
	return false
}

/*
	Loops through the tags found for the volume and calls `setTag`
	to add it via the AWS api
*/
func addAWSTags(awsTags string, awsVolumeID string, separator string, l log.Entry) {

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

	tags := strings.Split(awsTags, separator)

	tagLogger := log.WithFields(log.Fields{
		"volume": awsVolume,
		"region": awsRegion,
		"tags":   tags,
	})

	resp, err := svc.DescribeVolumes(params)
	if err != nil {
		tagLogger.WithError(err).Error("Cannot get volume")
		return
	}
	for i := range tags {
		tagLogger.Info("Adding tag to EBS Volume")
		t := strings.Split(tags[i], "=")
		if len(t) != 2 {
			tagLogger.WithFields(log.Fields{"tag": t}).Error("Skipping Malformed Tag")
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
		log.WithError(err).Fatal("Error creating tags")
		return false
	}
	if *debug {
		log.Debugf("Returned value from CreateTags call: %v", ret)
	}
	return true
}

/*
   Check if the tag is already set. It wouldn't be a problem if it is
   but if you're using cloudtrail it may be an issue seeing it
   being set all muiltiple times
*/
func hasTag(tags []*ec2.Tag, key string, value string) bool {
	for i := range tags {
		if *tags[i].Key == key && *tags[i].Value == value {
			log.WithFields(log.Fields{"key": key, "value": value}).Info("Tag value already exists")
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
