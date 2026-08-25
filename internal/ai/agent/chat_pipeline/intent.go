package chat_pipeline

import "strings"

func shouldRetrieve(query string) bool {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return false
	}
	for _, phrase := range []string{"你好", "您好", "hello", "hi", "谢谢", "再见", "现在几点", "几点了", "今天日期", "当前时间", "继续", "刚才", "上面", "这个怎么办", "那个怎么办"} {
		if normalized == phrase || strings.HasPrefix(normalized, phrase) {
			return false
		}
	}
	return true
}
