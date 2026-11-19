package service

import (
	"sync"
	"testing"
)

func TestAccountGetModelMappingConcurrentLazyCache(t *testing.T) {
	account := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{"model-a": "upstream-a"},
	}}
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			mapping := account.GetModelMapping()
			if got := mapping["model-a"]; got != "upstream-a" {
				t.Errorf("mapping = %q, want upstream-a", got)
			}
		}()
	}
	wg.Wait()
}
