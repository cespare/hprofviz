package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
)

var keep = flag.Int("keep", 50, "Only keep the top N most frequently sampled nodes")

type CallSite struct {
	Name       string
	Filename   string
	LineNumber int // -1 is 'unknown'
}

type Trace struct {
	ID    int
	Stack []*CallSite
	Count int
}

type Node struct {
	*CallSite
	Count       int
	EdgeWeights map[*Node]int // outbound
}

func (n *Node) String() string {
	return fmt.Sprintf("Node(%s){count=%d, edges=%v}", n.Name, n.Count, len(n.EdgeWeights))
}

// CreateNodes creates a new Node for each CallSite and hooks them together with weighted edges.
func CreateNodes(traces map[int]*Trace) []*Node {
	nodes := make(map[*CallSite]*Node)
	for _, trace := range traces {
		var child *Node
		for _, site := range trace.Stack {
			node, ok := nodes[site]
			if !ok {
				node = &Node{CallSite: site}
				nodes[site] = node
			}
			node.Count += trace.Count
			if child != nil {
				if node.EdgeWeights == nil {
					node.EdgeWeights = make(map[*Node]int)
				}
				node.EdgeWeights[child] += trace.Count
			}
			child = node
		}
	}
	var nodeList []*Node
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}
	return nodeList
}

type byWeight []*Node

func (w byWeight) Len() int           { return len(w) }
func (w byWeight) Less(i, j int) bool { return w[i].Count < w[j].Count }
func (w byWeight) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }

func Prune(nodes []*Node) []*Node {
	sort.Sort(sort.Reverse(byWeight(nodes)))
	topK := nodes[:*keep]
	pruned := make(map[*Node]bool)
	for _, toPrune := range nodes[*keep:] {
		pruned[toPrune] = true
	}
	for _, node := range topK {
		for edge := range node.EdgeWeights {
			if pruned[edge] {
				delete(node.EdgeWeights, edge)
			}
		}
		if len(node.EdgeWeights) == 0 {
			node.EdgeWeights = nil
		}
	}
	return topK
}

func main() {
	flag.Parse()
	if flag.NArg() != 2 {
		log.Fatal("Usage: hprofviz HPROF_FILE.txt OUTPUT_FILE.dot")
	}
	filename := flag.Arg(0)
	traces := ParseHProfFile(filename)
	nodes := CreateNodes(traces)
	f, err := os.Create(flag.Arg(1))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	nodes = Prune(nodes)
	if err := WriteDotFormat(f, filename, nodes); err != nil {
		log.Fatal(err)
	}
}
