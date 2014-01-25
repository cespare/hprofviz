# HProfviz

HProfviz turns  [HProf](http://docs.oracle.com/javase/7/docs/technotes/samples/hprof.html)'s CPU sampling
output into a graph in the [DOT language](http://en.wikipedia.org/wiki/DOT_(graph_description_language) in a
manner similar to [gperftools](https://code.google.com/p/gperftools).

## Installation

You'll need Go installed.

    $ go get github.com/cespare/hprofviz

You can run `hprofviz -h` to see how to invoke the tool.

## Usage

See the information on the HProf homepage. I recommend running with the stack depth turned high, so that you
capture complete stacks.

Here's an example of how you might run your application:

    $ java -agentlib:hprof=cpu=samples,interval=50,depth=150 -jar app.jar

Next, feed the dump into HProfViz:

    $ hprofviz java.hprof.txt hprof.dot

The second filename is the name of the DOT output file. You need Graphviz or another tool installed to render
the graph.

    $ dot -Tpng hprof.dot > hprof.png

## Notes

The graph can get busy if you have a large number of different samples or very complex code. I recommend
reading the top methods list at the end of the hprof output file (look for "CPU SAMPLES BEGIN") to gain a
better understanding of what the most expensive method calls are. Then you can use the hprofviz flags to limit
your view:

    $ hprofviz -topk 3 java.hprof.txt hprof.dot

This restricts the dataset to only include stack traces where the top 3 most expensive methods were being
called.

    $ hprofviz -regez 'Foo' java.hprof.txt hprof.dot

This restricts the dataset to only include stack traces where the method being called matches `/Foo/`.
