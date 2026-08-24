package main

import (
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	authn "SuperBizAgent/internal/auth"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/log_call_back"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
)

func main() {
	tenantID := os.Getenv("SUPERBIZ_KNOWLEDGE_TENANT_ID")
	if tenantID == "" {
		panic("SUPERBIZ_KNOWLEDGE_TENANT_ID is required")
	}
	userID := os.Getenv("SUPERBIZ_KNOWLEDGE_USER_ID")
	if userID == "" {
		userID = "knowledge-indexer"
	}
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{
		TenantID: tenantID,
		UserID:   userID,
		Username: userID,
	})
	r, err := knowledge_index_pipeline.BuildKnowledgeIndexing(ctx)
	if err != nil {
		panic(err)
	}
	err = filepath.WalkDir("./docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk dir failed: %w", err)
		}
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			fmt.Printf("[skip] not a markdown file: %s\n", path)
			return nil
		}

		fmt.Printf("[start] indexing file: %s\n", path)
		// 删除biz数据metadata中_source一样的数据
		loader, err := loader2.NewFileLoader(ctx)
		if err != nil {
			return err
		}
		docs, err := loader.Load(ctx, document.Source{URI: path})
		if err != nil {
			return err
		}
		cli, err := client.NewMilvusClient(ctx)
		if err != nil {
			return err
		}
		// 查询所有metadata中_source一样的数据并删除
		expr := tenantSourceFilterExpression(fmt.Sprint(docs[0].MetaData["_source"]), tenantID)
		queryResult, err := cli.Query(ctx, common.MilvusCollectionName, []string{}, expr, []string{"id"})
		if err != nil {
			return err
		} else if len(queryResult) > 0 {
			// 提取所有需要删除的id
			var idsToDelete []string
			for _, column := range queryResult {
				if column.Name() == "id" {
					for i := 0; i < column.Len(); i++ {
						id, err := column.GetAsString(i)
						if err == nil {
							idsToDelete = append(idsToDelete, id)
						}
					}
				}
			}
			// 删除这些数据
			if len(idsToDelete) > 0 {
				deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
				err = cli.Delete(ctx, common.MilvusCollectionName, "", deleteExpr)
				if err != nil {
					fmt.Printf("[warn] delete existing data failed: %v\n", err)
				} else {
					fmt.Printf("[info] deleted %d existing records with _source: %s\n", len(idsToDelete), docs[0].MetaData["_source"])
				}
			}
		}
		// 重新构建
		ids, err := r.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(log_call_back.LogCallback(nil)))
		if err != nil {
			return fmt.Errorf("invoke index graph failed: %w", err)
		}
		fmt.Printf("[done] indexing file: %s, len of parts: %d，%s\n", path, len(ids), ids)
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func sourceFilterExpression(source string) string {
	return fmt.Sprintf(`metadata["_source"] == %s`, strconv.Quote(filepath.ToSlash(source)))
}

func tenantSourceFilterExpression(source, tenantID string) string {
	return fmt.Sprintf(`metadata["_source"] == %s and metadata["tenant_id"] == %s`,
		strconv.Quote(filepath.ToSlash(source)), strconv.Quote(tenantID))
}
