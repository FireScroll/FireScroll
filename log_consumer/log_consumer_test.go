package log_consumer

import (
	"fmt"
	"github.com/FireScroll/FireScroll/utils"
	"github.com/twmb/franz-go/pkg/kgo"
	"testing"
)

func TestPartition(t *testing.T) {
	input := "blah"

	part := kgo.StickyKeyPartitioner(nil).ForTopic("blah").Partition(&kgo.Record{
		Key: []byte(input),
	}, 256)

	fmt.Println("part", part)

	partm := utils.Murmur2([]byte(input))
	fmt.Println("partm", partm%256)
}
