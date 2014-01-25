package main

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"text/template"
)

type DotNode struct {
	Num   int
	Label string
	Count int
}

type DotEdge struct {
	Node1, Node2 int // DotNode.Num
	Label        string
	Weight int
}

type DotGraph struct {
	Filename string
	MaxCount int
	Nodes    []*DotNode
	Edges    []*DotEdge
}

func WriteDotFormat(w io.Writer, filename string, nodes []*Node) error {
	totalCount := 0
	for _, node := range nodes {
		totalCount += node.Count
	}
	fmt.Printf("\033[01;34m>>>> totalCount: %v\x1B[m\n", totalCount)

	nodeToDotNode := make(map[*Node]*DotNode)
	var dotNodes []*DotNode
	num := 1
	for _, node := range nodes {
		lineNumber := "???"
		if node.LineNumber > 0 {
			lineNumber = strconv.Itoa(node.LineNumber)
		}
		selfFraction := float64(node.Count) / float64(totalCount)
		line := fmt.Sprintf(
			"%d (%0.1f%%) %s[%s:%s]", node.Count, 100*selfFraction, node.Name, node.Filename, lineNumber,
		)
		dotNode := &DotNode{
			Num:   num,
			Label: line,
			Count: node.Count,
		}
		num++
		nodeToDotNode[node] = dotNode
		dotNodes = append(dotNodes, dotNode)
	}

	var edges []*DotEdge
	for _, node := range nodes {
		for child, weight := range node.EdgeWeights {
			edge := &DotEdge{
				Node1: nodeToDotNode[node].Num,
				Node2: nodeToDotNode[child].Num,
				Label: fmt.Sprintf("%d (%.1f%%)", weight, 100*float64(weight)/float64(totalCount)),
				Weight: weight,
			}
			edges = append(edges, edge)
		}
	}

	graph := &DotGraph{
		Filename: filename,
		MaxCount: totalCount,
		Nodes:    dotNodes,
		Edges:    edges,
	}
	// These mysterious sizing functions are copied from pprof's perl script.
	fontSize := func(count int) float64 {
		return 50*math.Sqrt(float64(count)/float64(totalCount)) + 8
	}
	edgeWeight := func(weight int) int {
		w := math.Pow(float64(weight), 0.7)
		if w > 100000 {
			w = 100000
		}
		return int(w)
	}
	edgeWidth := func(weight int) float64 {
		f := 3 * (float64(weight) / float64(totalCount))
		if f > 1 {
			f = 1
		}
		w := f * 2
		if w < 1 {
			w = 1
		}
		return w
	}

	dotTemplate, err := template.New("dot").Funcs(map[string]interface{}{
		"fontSize": fontSize,
		"edgeWeight": edgeWeight,
		"edgeWidth": edgeWidth,
	}).Parse(tmpl)
	if err != nil {
		return err
	}
	return dotTemplate.Execute(w, graph)
}

var tmpl = `digraph "HProf output for {{.Filename}}" {
node [width=0.375,height=0.25];
Legend [shape=box,fontsize=24,shape=plaintext,label="{{.Filename}}:\lexamining {{.MaxCount}} samples"];
{{range .Nodes}}N{{.Num}} [label="{{.Label}}",shape=box,fontsize={{fontSize .Count | printf "%0.2f"}}];
{{end}}
{{range .Edges}}N{{.Node1}} -> N{{.Node2}} [label="{{.Label}}", weight={{edgeWeight .Weight}}, style="setlinewidth({{edgeWidth .Weight | printf "%.3f"}})"];
{{end}}
}
`
