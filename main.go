// SPDX-FileCopyrightText: 2022 Kalle Fagerberg
//
// SPDX-License-Identifier: GPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the
// Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package main is the entrypoint for this module.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/disiqueira/gotree"
	"github.com/jilleJr/hns-tree/internal/flagtypes"
	"github.com/spf13/pflag"
	"gopkg.in/typ.v4/slices"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var flags = struct {
	kubeconfig string
	showHelp   bool
	output     flagtypes.Output
}{
	output: flagtypes.OutputTree,
}

func init() {
	if home := homedir.HomeDir(); home != "" {
		pflag.StringVar(&flags.kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		pflag.StringVar(&flags.kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	pflag.BoolVarP(&flags.showHelp, "help", "h", false, "show this help text")
	pflag.VarP(&flags.output, "output", "o", "output format: tree, json, or yaml")
}

func main() {
	pflag.Parse()
	if flags.showHelp {
		pflag.Usage()
		os.Exit(0)
	}
	if err := mainErr(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		os.Exit(1)
	}
}

func mainErr() error {
	config, err := clientcmd.BuildConfigFromFlags("", flags.kubeconfig)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	nsClient := clientset.CoreV1().Namespaces()
	list, err := nsClient.List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	rootNodes := createTree(list.Items)
	switch flags.output {
	case flagtypes.OutputJSON:
		return printJSON(rootNodes)
	case flagtypes.OutputYAML:
		return printYAML(rootNodes)
	default:
		printTree(rootNodes)
		return nil
	}
}

// Node is a namespace tree node with a list of its children.
type Node struct {
	Name     string  `json:"name" yaml:"name"`
	Children []*Node `json:"children,omitempty" yaml:"children,omitempty"`
}

func createTree(namespaces []v1.Namespace) []Node {
	type NodeWithAnnotations struct {
		*Node
		Annotations map[string]string
	}
	nodesMap := make(map[string]NodeWithAnnotations)
	for _, ns := range namespaces {
		nodesMap[ns.Name] = NodeWithAnnotations{
			Node: &Node{
				Name: ns.Name,
			},
			Annotations: ns.Annotations,
		}
	}
	var childrenNames []string
	for _, node := range nodesMap {
		parentName, ok := node.Annotations["hnc.x-k8s.io/subnamespace-of"]
		if !ok {
			continue
		}
		parentNode := nodesMap[parentName]
		parentNode.Children = append(parentNode.Children, node.Node)
		childrenNames = append(childrenNames, node.Name)
	}
	var rootNodes []Node
	for _, node := range nodesMap {
		if !slices.Contains(childrenNames, node.Name) {
			rootNodes = append(rootNodes, *node.Node)
		}
	}
	for _, node := range nodesMap {
		slices.SortFunc(node.Children, func(a, b *Node) bool {
			return a.Name < b.Name
		})
	}
	slices.SortFunc(rootNodes, func(a, b Node) bool {
		return a.Name < b.Name
	})
	return rootNodes
}

func printTree(rootNodes []Node) {
	var buf bytes.Buffer
	for _, node := range rootNodes {
		tree := gotree.New(node.Name)
		addNodes(tree, node.Children)
		buf.WriteString(tree.Print())
	}
	buf.WriteTo(os.Stdout)
}

func addNodes(tree gotree.Tree, nodes []*Node) {
	for _, node := range nodes {
		subtree := tree.Add(node.Name)
		addNodes(subtree, node.Children)
	}
}

func printJSON(rootNodes []Node) error {
	b, err := json.MarshalIndent(rootNodes, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func printYAML(rootNodes []Node) error {
	b, err := yaml.Marshal(rootNodes)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
