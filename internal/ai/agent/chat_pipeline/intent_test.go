package chat_pipeline

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

func TestIntentRouterSkipsNonKnowledgeQueries(t *testing.T) {
	for _, query := range []string{"你好", "hello", "现在几点", "今天日期是什么", "继续", "刚才那个怎么办"} {
		if got := shouldRetrieve(query); got {
			t.Errorf("query %q should skip retrieval", query)
		}
	}
}

func TestIntentRouterRetrievesOperationalQueries(t *testing.T) {
	for _, query := range []string{"api 502 错误怎么处理", "查询 ap-guangzhou 的告警日志", "服务离线的处理手册"} {
		if got := shouldRetrieve(query); !got {
			t.Errorf("query %q should retrieve knowledge", query)
		}
	}
}

func TestIntentRouterDefaultsUnknownQueriesToRetrieval(t *testing.T) {
	if !shouldRetrieve("如何优化这个系统") {
		t.Fatal("unknown knowledge query should retrieve by default")
	}
}

func TestIntentAwareRetrieverSkipsDelegateForGreeting(t *testing.T) {
	delegate := &intentTestRetriever{}
	wrapper := &intentAwareRetriever{delegate: delegate}
	docs, err := wrapper.Retrieve(context.Background(), "你好")
	if err != nil || len(docs) != 0 {
		t.Fatalf("greeting result=%v err=%v", docs, err)
	}
	if delegate.calls != 0 {
		t.Fatalf("delegate calls=%d, want 0", delegate.calls)
	}
}

type intentTestRetriever struct{ calls int }

func (r *intentTestRetriever) Retrieve(context.Context, string, ...retriever.Option) ([]*schema.Document, error) {
	r.calls++
	return []*schema.Document{{ID: "doc"}}, nil
}
