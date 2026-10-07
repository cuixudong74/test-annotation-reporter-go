// testreport 归并多个分片的测试结果并输出统一报告与 CI 注解。
//
// 用法:
//
//	testreport -format gotest -platform github -max 50 shard1.json shard2.xml ...
//
// 每个输入文件可用 "路径:格式" 指定各自格式（gotest|junit），
// 未指定时使用 -format。报告写到 stdout，注解写到 -annotations 指定的文件
// （默认 stderr）。被拒绝的记录逐条打印到 stderr 前缀 "REJECT"。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cuixudong74/test-annotation-reporter-go/reporter"
)

func main() {
	formatFlag := flag.String("format", "gotest", "默认输入格式: gotest|junit")
	platformFlag := flag.String("platform", "github", "注解平台: github|azure|gitlab")
	maxFlag := flag.Int("max", 0, "注解数量上限，0 表示不限制")
	annFile := flag.String("annotations", "", "注解输出文件（默认 stderr）")
	flag.Parse()

	defFormat, err := parseFormat(*formatFlag)
	if err != nil {
		fatal(err)
	}
	platform, err := parsePlatform(*platformFlag)
	if err != nil {
		fatal(err)
	}

	merger := reporter.NewMerger()
	rejected := 0
	for _, arg := range flag.Args() {
		path, format := arg, defFormat
		if i := strings.LastIndex(arg, ":"); i > 0 {
			if f, err := parseFormat(arg[i+1:]); err == nil {
				path, format = arg[:i], f
			}
		}
		f, err := os.Open(path)
		if err != nil {
			fatal(err)
		}
		ch := make(chan reporter.Record, 256)
		go func() {
			defer close(ch)
			if err := reporter.ParseStream(format, path, f, ch); err != nil {
				fmt.Fprintf(os.Stderr, "REJECT %s: %v\n", path, err)
			}
			f.Close()
		}()
		rejected += merger.Ingest(ch, func(e *reporter.RejectError) {
			fmt.Fprintf(os.Stderr, "REJECT %s: %v\n", path, e)
		})
	}

	rep := merger.Snapshot()
	if err := rep.WriteText(os.Stdout); err != nil {
		fatal(err)
	}

	var aw io.Writer = os.Stderr
	if *annFile != "" {
		f, err := os.Create(*annFile)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		aw = f
	}
	ann := reporter.Annotator{Platform: platform, Max: *maxFlag}
	if err := ann.WriteAnnotations(rep, aw); err != nil {
		fatal(err)
	}
	if rep.Counts[reporter.StatusFailed]+rep.Counts[reporter.StatusError] > 0 {
		os.Exit(1)
	}
}

func parseFormat(s string) (reporter.Format, error) {
	switch s {
	case "gotest":
		return reporter.FormatGoTestJSON, nil
	case "junit":
		return reporter.FormatJUnitXML, nil
	}
	return 0, fmt.Errorf("unknown format %q", s)
}

func parsePlatform(s string) (reporter.Platform, error) {
	switch s {
	case "github":
		return reporter.PlatformGitHub, nil
	case "azure":
		return reporter.PlatformAzure, nil
	case "gitlab":
		return reporter.PlatformGitLab, nil
	}
	return 0, fmt.Errorf("unknown platform %q", s)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "testreport:", err)
	os.Exit(2)
}
