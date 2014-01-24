package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	traceLine      = regexp.MustCompile(`^([^\(]+)\(([^\:]+):([^\)]+)\)$`)
	traceHeader    = regexp.MustCompile(`^TRACE (\d+):$`)
	samplesHeader  = regexp.MustCompile(`^CPU SAMPLES BEGIN \(total = (\d+)\)`)
	samplesColumns = regexp.MustCompile(`^rank\s+self\s+accum\s+count\s+trace\s+method$`)
)

func ParseHProfFile(filename string) map[int]*Trace {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}

	lineNumber := 0
	parseError := func(args ...interface{}) {
		argList := append([]interface{}{fmt.Sprintf("Line %d: ", lineNumber)}, args...)
		log.Fatalln(argList...)
	}
	parseErrorf := func(format string, args ...interface{}) {
		log.Fatalf(fmt.Sprintf("Line %d: ", lineNumber)+format, args...)
	}
	traces := make(map[int]*Trace)          // by ID
	callSites := make(map[string]*CallSite) // by line (stripped of leading \t)
	var currentTrace *Trace
	scanner := bufio.NewScanner(f)
	inTrace := false
	inSamples := false
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if inTrace && !strings.HasPrefix(line, "\t") {
			inTrace = false
		}

		// Header lines, threads, etc
		if !inTrace && !inSamples {
			traceHeaderParts := traceHeader.FindStringSubmatch(line)
			if traceHeaderParts != nil {
				inTrace = true
				id, err := strconv.Atoi(traceHeaderParts[1])
				if err != nil {
					parseError("cannot parse TRACE line")
				}
				currentTrace = &Trace{ID: id}
				if _, ok := traces[id]; ok {
					parseError("duplicate trace with id", id)
				}
				traces[id] = currentTrace
				continue
			}
			if samplesHeader.MatchString(line) {
				inSamples = true
			}
			continue
		}

		if inTrace {
			line = strings.TrimPrefix(line, "\t") // We already know the line has a \t prefix
			callSite, ok := callSites[line]
			if !ok {
				traceLineParts := traceLine.FindStringSubmatch(line)
				if len(traceLineParts) != 4 {
					parseError("cannot parse trace line")
				}
				var n int
				n, err := strconv.Atoi(traceLineParts[3])
				if err != nil {
					if traceLineParts[3] == "Unknown line" {
						n = -1
					} else {
						parseError("bad line number")
					}
				}
				callSite = &CallSite{
					Name:       traceLineParts[1],
					Filename:   traceLineParts[2],
					LineNumber: n,
				}
				callSites[line] = callSite
			}
			currentTrace.Stack = append(currentTrace.Stack, callSite)
			continue
		}

		if inSamples {
			if strings.HasPrefix(line, " ") {
				fields := strings.Fields(line)
				if len(fields) != 6 {
					parseError("unexpected number of columns")
				}
				count, err := strconv.Atoi(fields[3])
				if err != nil {
					parseError("cannot parse count")
				}
				id, err := strconv.Atoi(fields[4])
				if err != nil {
					parseError("cannot parse id")
				}
				trace := traces[id]
				if trace == nil {
					parseErrorf("found id %d, but no trace with such id exists", id)
				}
				trace.Count = count
			}
			if samplesColumns.MatchString(line) {
				continue
			}
			if line == "CPU SAMPLES END" {
				inSamples = false
				continue
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return traces
}
