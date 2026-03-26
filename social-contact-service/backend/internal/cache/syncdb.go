package cache

import (
	"context"
	"log"
	"sync"

	"social-contact-service/backend/internal/repo"
)

// 同步缓存中的文档统计数据到 MySQL
func SyncStatsToMySQL(ctx context.Context, interactionRepo repo.InteractionRepo, docStatsRepo repo.DocStatsRepo) error {
	docs, err := interactionRepo.GetDocs(ctx)
	if err != nil {
		return err
	}
	wg := sync.WaitGroup{}
	for _, doc := range docs {
		wg.Add(1)
		go func(doc string) {
			defer wg.Done()
			stats, err := interactionRepo.GetDocStats(ctx, doc)
			if err != nil {
				log.Printf("syncdb get doc stats failed: %v", err)
			}
			if stats == nil {
				log.Printf("syncdb doc stats not found: %s", doc)
			}
			err = docStatsRepo.SetDocStats(ctx, doc, stats)
			if err != nil {
				log.Printf("syncdb set doc stats failed: %v", err)
			}
		}(doc)
	}
	wg.Wait()
	return nil
}
