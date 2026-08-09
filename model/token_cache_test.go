package model

import (
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

type memoryTokenCacheBackend struct {
	mu              sync.Mutex
	generationValue int64
	cache           map[string]Token
	beforeSet       func(Token, int64)
}

func newMemoryTokenCacheBackend() *memoryTokenCacheBackend {
	return &memoryTokenCacheBackend{
		cache: make(map[string]Token),
	}
}

func (backend *memoryTokenCacheBackend) generation() (int64, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.generationValue, nil
}

func (backend *memoryTokenCacheBackend) setTokenIfGeneration(
	token Token,
	generation int64,
	preserveCachedQuota bool,
) (bool, error) {
	if backend.beforeSet != nil {
		backend.beforeSet(token, generation)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.generationValue != generation {
		return false, nil
	}
	if preserveCachedQuota {
		if cached, exists := backend.cache[token.Key]; exists {
			token.RemainQuota = cached.RemainQuota
		}
	}
	backend.cache[token.Key] = token
	return true, nil
}

func (backend *memoryTokenCacheBackend) invalidateTokens(keys []string) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.generationValue++
	for _, key := range uniqueTokenCacheKeys(keys) {
		delete(backend.cache, key)
	}
	return nil
}

func (backend *memoryTokenCacheBackend) invalidateTokenIfGeneration(
	key string,
	generation int64,
) (bool, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.generationValue != generation {
		return false, nil
	}
	backend.generationValue++
	delete(backend.cache, key)
	return true, nil
}

func (backend *memoryTokenCacheBackend) get(key string) (Token, bool) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	token, exists := backend.cache[key]
	return token, exists
}

func waitTokenCacheTestSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("等待%s超时", name)
	}
}

func waitTokenCacheTestResult(t *testing.T, result <-chan error, name string) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("%s失败: %v", name, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("等待%s完成超时", name)
	}
}

func setTokenGroupAutoForCacheTest(tokenID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
			"group":              TokenGroupModeAuto,
			"group_mode":         TokenGroupModeAuto,
			"group_ratio_limits": "",
		}).Error; err != nil {
			return err
		}
		return tx.Where("token_id = ?", tokenID).Delete(&TokenGroupBinding{}).Error
	})
}

func TestTokenCacheInvalidationRejectsOlderRefresh(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         701,
		Key:            "cache-refresh-before-migration",
		Name:           "cache-refresh-before-migration",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建缓存并发测试令牌失败: %v", err)
	}

	backend := newMemoryTokenCacheBackend()
	setStarted := make(chan struct{})
	allowSet := make(chan struct{})
	var setOnce sync.Once
	backend.beforeSet = func(Token, int64) {
		setOnce.Do(func() {
			close(setStarted)
			<-allowSet
		})
	}
	refreshResult := make(chan error, 1)
	go func() {
		refreshResult <- cacheRefreshTokenWithBackend(backend, token.Id, token.Key, true)
	}()
	waitTokenCacheTestSignal(t, setStarted, "旧缓存刷新完成数据库读取")

	// 即使旧任务在读库后长时间暂停，迁移仍可提交并通过 generation 使其失效。
	if err := setTokenGroupAutoForCacheTest(token.Id); err != nil {
		t.Fatalf("在缓存写入暂停期间更新数据库失败: %v", err)
	}
	if err := cacheDeleteTokenWithBackend(backend, token.Key); err != nil {
		t.Fatalf("迁移缓存失效失败: %v", err)
	}
	close(allowSet)
	waitTokenCacheTestResult(t, refreshResult, "旧缓存刷新")
	if cached, exists := backend.get(token.Key); exists {
		t.Fatalf("旧 generation 仍写回了过期分组缓存: %#v", cached)
	}
}

func TestTokenCacheRefreshReadsNewStateAfterInvalidation(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         702,
		Key:            "cache-migration-before-refresh",
		Name:           "cache-migration-before-refresh",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建缓存并发测试令牌失败: %v", err)
	}
	backend := newMemoryTokenCacheBackend()
	backend.cache[token.Key] = *token
	if err := setTokenGroupAutoForCacheTest(token.Id); err != nil {
		t.Fatalf("设置迁移后的令牌状态失败: %v", err)
	}
	if err := cacheDeleteTokenWithBackend(backend, token.Key); err != nil {
		t.Fatalf("迁移缓存失效失败: %v", err)
	}
	if err := cacheRefreshTokenWithBackend(backend, token.Id, token.Key, true); err != nil {
		t.Fatalf("迁移后缓存刷新失败: %v", err)
	}
	cached, exists := backend.get(token.Key)
	if !exists {
		t.Fatal("迁移后刷新未回填令牌缓存")
	}
	if cached.Group != TokenGroupModeAuto || cached.GroupMode != TokenGroupModeAuto || len(cached.GroupIds) != 0 {
		t.Fatalf("迁移后刷新回写了旧分组状态: %#v", cached)
	}
}

func TestTokenCacheStaleGenerationIsRejected(t *testing.T) {
	backend := newMemoryTokenCacheBackend()
	key := "cache-stale-generation"
	generation, err := backend.generation()
	if err != nil {
		t.Fatalf("读取初始 generation 失败: %v", err)
	}
	if err := backend.invalidateTokens([]string{key}); err != nil {
		t.Fatalf("提升 generation 失败: %v", err)
	}
	written, err := backend.setTokenIfGeneration(Token{Key: key, Group: "old"}, generation, false)
	if err != nil {
		t.Fatalf("尝试写入旧 generation 失败: %v", err)
	}
	if written {
		t.Fatal("旧 generation 的缓存写入未被拒绝")
	}
	if _, exists := backend.get(key); exists {
		t.Fatal("拒绝后仍生成了缓存")
	}
}

func TestTokenCacheBatchInvalidationBumpsGlobalGenerationOnce(t *testing.T) {
	backend := newMemoryTokenCacheBackend()
	backend.cache["token-a"] = Token{Key: "token-a"}
	backend.cache["token-b"] = Token{Key: "token-b"}

	before, err := backend.generation()
	if err != nil {
		t.Fatalf("读取失效前 generation 失败: %v", err)
	}
	if err := backend.invalidateTokens([]string{"token-a", "token-b", "token-a"}); err != nil {
		t.Fatalf("批量失效令牌缓存失败: %v", err)
	}
	after, err := backend.generation()
	if err != nil {
		t.Fatalf("读取失效后 generation 失败: %v", err)
	}
	if after != before+1 {
		t.Fatalf("批量失效未只提升一次全局 generation: before=%d after=%d", before, after)
	}
	if _, exists := backend.get("token-a"); exists {
		t.Fatal("token-a 缓存未删除")
	}
	if _, exists := backend.get("token-b"); exists {
		t.Fatal("token-b 缓存未删除")
	}

	written, err := backend.setTokenIfGeneration(Token{Key: "unrelated-token"}, before, false)
	if err != nil {
		t.Fatalf("检查其他令牌旧刷新失败: %v", err)
	}
	if written {
		t.Fatal("全局 generation 提升后仍接受了其他令牌的旧刷新")
	}
}

func TestTokenCacheRefreshPreservesLiveQuotaButUpdateCanReplaceIt(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         703,
		Key:            "cache-refresh-preserve-live-quota",
		Name:           "cache-refresh-preserve-live-quota",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		RemainQuota:    100,
		UnlimitedQuota: false,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建额度保留测试令牌失败: %v", err)
	}
	backend := newMemoryTokenCacheBackend()
	backend.cache[token.Key] = Token{Key: token.Key, RemainQuota: 70}
	if err := cacheRefreshTokenWithBackend(backend, token.Id, token.Key, true); err != nil {
		t.Fatalf("保留实时额度的缓存刷新失败: %v", err)
	}
	if cached, _ := backend.get(token.Key); cached.RemainQuota != 70 {
		t.Fatalf("普通数据库回填覆盖了实时额度: got=%d want=70", cached.RemainQuota)
	}
	if err := DB.Model(&Token{}).Where("id = ?", token.Id).Update("remain_quota", 120).Error; err != nil {
		t.Fatalf("更新权威令牌额度失败: %v", err)
	}
	if err := cacheRefreshTokenWithBackend(backend, token.Id, token.Key, false); err != nil {
		t.Fatalf("覆盖额度的缓存刷新失败: %v", err)
	}
	if cached, _ := backend.get(token.Key); cached.RemainQuota != 120 {
		t.Fatalf("权威令牌更新未覆盖额度: got=%d want=120", cached.RemainQuota)
	}
}
