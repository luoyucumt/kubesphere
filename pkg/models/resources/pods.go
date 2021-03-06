/*

 Copyright 2019 The KubeSphere Authors.

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
package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	"kubesphere.io/kubesphere/pkg/constants"
	"kubesphere.io/kubesphere/pkg/informers"
	"kubesphere.io/kubesphere/pkg/server/params"
	"kubesphere.io/kubesphere/pkg/utils/sliceutil"
	"sort"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type podSearcher struct {
}

func (*podSearcher) get(namespace, name string) (interface{}, error) {
	return informers.SharedInformerFactory().Core().V1().Pods().Lister().Pods(namespace).Get(name)
}

func podBelongTo(item *v1.Pod, kind string, name string) bool {
	switch kind {
	case "Deployment":
		if podBelongToDeployment(item, name) {
			return true
		}
	case "ReplicaSet":
		if podBelongToReplicaSet(item, name) {
			return true
		}
	case "DaemonSet":
		if podBelongToDaemonSet(item, name) {
			return true
		}
	case "StatefulSet":
		if podBelongToStatefulSet(item, name) {
			return true
		}
	case "Job":
		if podBelongToJob(item, name) {
			return true
		}
	}
	return false
}

func replicaSetBelongToDeployment(replicaSet *appsv1.ReplicaSet, deploymentName string) bool {
	for _, owner := range replicaSet.OwnerReferences {
		if owner.Kind == "Deployment" && owner.Name == deploymentName {
			return true
		}
	}
	return false
}

func podBelongToDaemonSet(item *v1.Pod, name string) bool {
	for _, owner := range item.OwnerReferences {
		if owner.Kind == "DaemonSet" && owner.Name == name {
			return true
		}
	}
	return false
}

func podBelongToJob(item *v1.Pod, name string) bool {
	for _, owner := range item.OwnerReferences {
		if owner.Kind == "Job" && owner.Name == name {
			return true
		}
	}
	return false
}

func podBelongToReplicaSet(item *v1.Pod, replicaSetName string) bool {
	for _, owner := range item.OwnerReferences {
		if owner.Kind == "ReplicaSet" && owner.Name == replicaSetName {
			return true
		}
	}
	return false
}

func podBelongToStatefulSet(item *v1.Pod, statefulSetName string) bool {
	for _, owner := range item.OwnerReferences {
		if owner.Kind == "StatefulSet" && owner.Name == statefulSetName {
			return true
		}
	}
	return false
}

func podBelongToDeployment(item *v1.Pod, deploymentName string) bool {
	replicas, err := informers.SharedInformerFactory().Apps().V1().ReplicaSets().Lister().ReplicaSets(item.Namespace).List(labels.Everything())
	if err != nil {
		return false
	}

	for _, r := range replicas {
		if replicaSetBelongToDeployment(r, deploymentName) && podBelongToReplicaSet(item, r.Name) {
			return true
		}
	}

	return false
}

func podBindPVC(item *v1.Pod, pvcName string) bool {
	for _, v := range item.Spec.Volumes {
		if v.VolumeSource.PersistentVolumeClaim != nil &&
			v.VolumeSource.PersistentVolumeClaim.ClaimName == pvcName {
			return true
		}
	}
	return false
}

func podBelongToService(item *v1.Pod, serviceName string) bool {
	service, err := informers.SharedInformerFactory().Core().V1().Services().Lister().Services(item.Namespace).Get(serviceName)
	if err != nil {
		return false
	}

	selector := labels.Set(service.Spec.Selector).AsSelectorPreValidated()
	if selector.Empty() || !selector.Matches(labels.Set(item.Labels)) {
		return false
	}
	return true
}

// exactly Match
func (*podSearcher) match(match map[string]string, item *v1.Pod) bool {
	for k, v := range match {
		switch k {
		case OwnerKind:
			fallthrough
		case OwnerName:
			kind := match[OwnerKind]
			name := match[OwnerName]
			if !podBelongTo(item, kind, name) {
				return false
			}
		case "nodeName":
			if item.Spec.NodeName != v {
				return false
			}
		case "pvcName":
			if !podBindPVC(item, v) {
				return false
			}
		case "serviceName":
			if !podBelongToService(item, v) {
				return false
			}
		case Name:
			names := strings.Split(v, "|")
			if !sliceutil.HasString(names, item.Name) {
				return false
			}
		case Keyword:
			if !strings.Contains(item.Name, v) && !searchFuzzy(item.Labels, "", v) && !searchFuzzy(item.Annotations, "", v) {
				return false
			}
		default:
			// label not exist or value not equal
			if val, ok := item.Labels[k]; !ok || val != v {
				return false
			}
		}
	}
	return true
}

// Fuzzy searchInNamespace
func (*podSearcher) fuzzy(fuzzy map[string]string, item *v1.Pod) bool {
	for k, v := range fuzzy {
		switch k {
		case Name:
			if !strings.Contains(item.Name, v) && !strings.Contains(item.Annotations[constants.DisplayNameAnnotationKey], v) {
				return false
			}
		case Label:
			if !searchFuzzy(item.Labels, "", v) {
				return false
			}
		case annotation:
			if !searchFuzzy(item.Annotations, "", v) {
				return false
			}
			return false
		case app:
			if !strings.Contains(item.Labels[chart], v) && !strings.Contains(item.Labels[release], v) {
				return false
			}
		default:
			if !searchFuzzy(item.Labels, k, v) {
				return false
			}
		}
	}
	return true
}

func (*podSearcher) compare(a, b *v1.Pod, orderBy string) bool {
	switch orderBy {
	case CreateTime:
		return a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time)
	case Name:
		fallthrough
	default:
		return strings.Compare(a.Name, b.Name) <= 0
	}
}

func (s *podSearcher) search(namespace string, conditions *params.Conditions, orderBy string, reverse bool) ([]interface{}, error) {

	pods, err := informers.SharedInformerFactory().Core().V1().Pods().Lister().Pods(namespace).List(labels.Everything())

	if err != nil {
		return nil, err
	}

	result := make([]*v1.Pod, 0)

	if len(conditions.Match) == 0 && len(conditions.Fuzzy) == 0 {
		result = pods
	} else {
		for _, item := range pods {
			if s.match(conditions.Match, item) && s.fuzzy(conditions.Fuzzy, item) {
				result = append(result, item)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if reverse {
			tmp := i
			i = j
			j = tmp
		}
		return s.compare(result[i], result[j], orderBy)
	})

	r := make([]interface{}, 0)
	for _, i := range result {
		r = append(r, i)
	}
	return r, nil
}
