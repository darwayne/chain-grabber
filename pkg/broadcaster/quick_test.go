package broadcaster

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strconv"
	"strings"
	"testing"
)

func TestGen(t *testing.T) {
	var times [][][2]string
	err := json.Unmarshal([]byte(str), &times)
	require.NoError(t, err)

	var builder strings.Builder
	for idx, item := range times {
		start := item[0][0]
		end := item[0][1]
		total := timeDiff(start, end)
		fmt.Println("diff is", total, "for", idx, "start:", start, "end:", end)
		builder.WriteString(
			fmt.Sprintf("ffmpeg -ss 00:%s -i input.mp4 -t %s -c copy output_clip%d.mp4;",
				start, total, idx))
	}

	commands := "rm files.txt;" + builder.String()
	builder.Reset()

	for idx := range times {
		builder.WriteString(
			fmt.Sprintf(`echo "file 'output_clip%d.mp4'" >> files.txt;`, idx))
	}

	fmt.Printf(`%s%sffmpeg -f concat -safe 0 -i files.txt -c copy final.mp4;rm output_clip*`+"\n", commands, builder.String())

}

func TestMerge(t *testing.T) {
	var times [][][2]string
	err := json.Unmarshal([]byte(str), &times)
	require.NoError(t, err)

	var builder strings.Builder
	for idx := range times {
		builder.WriteString(
			fmt.Sprintf("concat:output_clip%d.mp4", idx))
		if idx < len(times)-1 {
			builder.WriteString("|")
		}
	}

	fmt.Printf(`ffmpeg -i "%s" -c copy final.mp4`, builder.String())

}

func timeDiff(start, end string) string {
	return fmt.Sprintf("%d", convertToSeconds(end)-convertToSeconds(start))
}

func convertToSeconds(format string) int {
	format = strings.TrimPrefix(format, "0")
	pieces := strings.Split(format, ":")
	if len(pieces) != 2 {
		panic("ruh oh")
	}

	minutes, _ := strconv.Atoi(pieces[0])
	seconds, _ := strconv.Atoi(pieces[1])

	return (minutes * 60) + seconds
}

const str = `
[
[["02:51", "07:47"]],
[["08:10", "08:45"]],
[["09:41", "10:05"]],
[["10:13", "10:28"]],
[["10:37", "10:41"]],
[["11:52", "14:12"]],
[["14:16", "14:27"]],
[["14:32", "15:28"]],
[["15:37", "15:56"]],
[["16:05", "17:16"]],
[["17:27", "18:01"]],
[["18:08", "22:13"]],
[["23:13", "27:08"]]
]`
