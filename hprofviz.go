package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
)

var (
	topk  = flag.Int("topk", -1, "Only keep the top k most frequently sampled nodes and their ancestors")
	regex = flag.String("regex", "", "Only keep matching sampled nodes and their ancestors")
)

type CallSite struct {
	Name            string
	Filename        string
	LineNumber      int // -1 is 'unknown'
	Count           int
	CumulativeCount int
}

type Trace struct {
	ID    int
	Stack []*CallSite
	Count int
}

type byCount []*Trace

func (w byCount) Len() int           { return len(w) }
func (w byCount) Less(i, j int) bool { return w[i].Count < w[j].Count }
func (w byCount) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }

func FilterTopK(traces map[*Trace]bool, k int) {
	var orderedTraces []*Trace
	for trace := range traces {
		orderedTraces = append(orderedTraces, trace)
	}
	sort.Sort(sort.Reverse(byCount(orderedTraces)))
	for _, trace := range orderedTraces[k:] {
		delete(traces, trace)
	}
}

func FilterMatching(traces map[*Trace]bool, regex *regexp.Regexp) {
	for trace := range traces {
		if !regex.MatchString(trace.Stack[0].Name) {
			delete(traces, trace)
		}
	}
}

// A Node may represent a collapsed chain of multiple calls.
type Node struct {
	*CallSite
	EdgeWeights map[*Node]int // outbound
	BackLinks   map[*Node]bool
}

// CreateNodes creates a new Node for each CallSite and hooks them together with weighted edges. It also
// attaches counts to CallSites from the Trace they were in.
func CreateNodes(traces map[*Trace]bool) []*Node {
	nodes := make(map[*CallSite]*Node)
	for trace := range traces {
		var child *Node
		for i, site := range trace.Stack {
			node, ok := nodes[site]
			if !ok {
				node = &Node{CallSite: site}
				nodes[site] = node
			}
			if i == 0 {
				node.Count += trace.Count
			}
			node.CumulativeCount += trace.Count
			if child != nil {
				if node.EdgeWeights == nil {
					node.EdgeWeights = make(map[*Node]int)
				}
				node.EdgeWeights[child] += trace.Count
				if child.BackLinks == nil {
					child.BackLinks = make(map[*Node]bool)
				}
				child.BackLinks[node] = true
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
func CountSum(traces map[*Trace]bool) int {
	sum := 0
	for trace := range traces {
		sum += trace.Count
	}
	return sum
}

func frac(p, q int) string {
	return fmt.Sprintf("%d/%d (%.2f%%)", p, q, 100*float64(p)/float64(q))
}

func main() {
	flag.Parse()
	if *topk > 0 && *regex != "" {
		log.Fatal("Cannot provide both -topk and -regexp.")
	}
	flag.Usage = func() {
		fmt.Println("Usage: hprofviz [OPTIONS] HPROF_FILE.txt OUTPUT_FILE.dot\nwhere OPTIONS are:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if flag.NArg() != 2 {
		flag.Usage()
	}
	filename := flag.Arg(0)
	traces := ParseHProfFile(filename)

	if *topk > 0 {
		countBefore := CountSum(traces)
		FilterTopK(traces, *topk)
		fmt.Printf("Keeping %s of samples after filtering top %d most frequently sampled\n",
			frac(CountSum(traces), countBefore), *topk)
	}
	if *regex != "" {
		reg, err := regexp.Compile(*regex)
		if err != nil {
			log.Fatal(err)
		}
		countBefore := CountSum(traces)
		FilterMatching(traces, reg)
		fmt.Printf("Keeping %s of samples after filtering matching samples\n",
			frac(CountSum(traces), countBefore))
	}

	nodes := CreateNodes(traces)
	fmt.Printf("%d nodes for rendering\n", len(nodes))

	f, err := os.Create(flag.Arg(1))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := WriteDotFormat(f, filename, nodes); err != nil {
		log.Fatal(err)
	}
}
