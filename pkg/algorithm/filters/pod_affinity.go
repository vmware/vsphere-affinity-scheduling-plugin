/*
Copyright (c) 201８ VMware, Inc. All Rights Reserved.

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

package filters

import (
	"log"

	"github.com/vmware/vsphere-affinity-scheduling-plugin/pkg/algorithm"
	"github.com/vmware/vsphere-affinity-scheduling-plugin/pkg/constants"
	"github.com/vmware/vsphere-affinity-scheduling-plugin/pkg/selector"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HostTopologyKey is the TopologyKey used to indicate the scope of Affinity
// and AntiAffinity rule is physical host.
const HostTopologyKey = constants.HostLabel

type podAffinity struct {
	podLister algorithm.PodLister
	hostCache algorithm.HostCache
}

// NewPodAffinity creates a pod anti-affinity filter
func NewPodAffinity(podLister algorithm.PodLister, hostCache algorithm.HostCache) algorithm.Filter {
	return &podAffinity{
		podLister: podLister,
		hostCache: hostCache,
	}
}

func (p *podAffinity) Filter(pod *v1.Pod, nodes []string) ([]string, error) {
	log.Printf("apply podAffinity filter node: %s", nodes)
	if pod.Spec.Affinity == nil ||
		pod.Spec.Affinity.PodAffinity == nil ||
		pod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return nodes, nil
	}

	filtered := []string{}
	affinities := pod.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution

	// Get pod selector
	var selectors selector.And
	for _, af := range affinities {
		if af.TopologyKey != HostTopologyKey {
			continue
		}

		selector, err := metav1.LabelSelectorAsSelector(af.LabelSelector)
		if err != nil {
			return filtered, err
		}

		selectors = append(selectors, selector)
	}

	// Select all matched pods
	pods, err := p.podLister.ListPod(selectors)
	if err != nil {
		return filtered, err
	}

	if len(pods) == 0 {
		// no matching pods means no matching nodes
		return filtered, nil
	}

	// Find valid hosts
	nodeMap := make(map[string]struct{})
	for _, pod := range pods {
		if node := pod.Spec.NodeName; node != "" {
			nodeMap[node] = struct{}{}
		}
	}

	hostMap := make(map[string]struct{})
	for node := range nodeMap {
		h := p.hostCache.GetHost(node)
		hostMap[h] = struct{}{}
	}

	for host := range hostMap {
		for _, node := range p.hostCache.GetNodes(host) {
			nodeMap[node] = struct{}{}
		}
	}

	for _, node := range nodes {
		if _, ok := nodeMap[node]; ok {
			filtered = append(filtered, node)
		}
	}

	log.Printf("applied podAffinity filter node: %s", filtered)

	return filtered, nil
}
