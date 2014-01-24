package main

import (
	"fmt"
	"io"
	"strconv"
	"text/template"
)

type DotNode struct {
	Num   int
	Label string
}

type DotEdge struct {
	Node1, Node2 int // DotNode.Num
	Label        string
}

type DotGraph struct {
	Filename string
	Nodes    []*DotNode
	Edges    []*DotEdge
}

func WriteDotFormat(w io.Writer, filename string, nodes []*Node) error {
	nodeToDotNode := make(map[*Node]*DotNode)
	var dotNodes []*DotNode
	num := 1
	for _, node := range nodes {
		lineNumber := "???"
		if node.LineNumber > 0 {
			lineNumber = strconv.Itoa(node.LineNumber)
		}
		dotNode := &DotNode{
			Num:   num,
			Label: fmt.Sprintf("(%d) %s[%s:%s]", node.Count, node.Name, node.Filename, lineNumber),
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
				Label: strconv.Itoa(weight),
			}
			edges = append(edges, edge)
		}
	}

	graph := &DotGraph{
		Filename: filename,
		Nodes:    dotNodes,
		Edges:    edges,
	}
	return DotTemplate.Execute(w, graph)
}

var DotTemplate = template.Must(template.New("dot").Parse(tmpl))

var tmpl = `digraph "HProf output for {{.Filename}}" {
node [width=0.375,height=0.25];
Legend [shape=box,fontsize=24,shape=plaintext,label="some\linfo\lhere\l"];
{{range .Nodes}}N{{.Num}} [label="{{.Label}}",shape=box,fontsize=12.0];
{{end}}
{{range .Edges}}N{{.Node1}} -> N{{.Node2}} [label={{.Label}}, weight=10, style="setlinewidth(0.1)"];
{{end}}
}
`
