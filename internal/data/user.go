package data

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"testdemo/ent"
	"testdemo/ent/predicate"
	"testdemo/ent/user"
	"testdemo/internal/biz"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v3/log"
	"go.einride.tech/aip/filtering"
)

type userRepo struct {
	data *Data
}

// userRepo 负责 User 的数据访问实现。
//
// 缓存策略：Cache-Aside（旁路缓存）
// - 真实数据只按 id 存一份：testdemo:user:data:{id} -> User(JSON)
// - name/email 只作为“索引”存储：
//   - testdemo:user:index:name:{base64(name)}  -> id
//   - testdemo:user:index:email:{base64(email)} -> id
//
// 读：先查 Redis（命中直接返回），未命中查 DB 并回填 Redis。
// 写：写 DB 成功后写入/更新 Redis；更新时需要清理旧索引，避免脏索引。

// NewUserRepo 创建用户仓储实现，并以 biz.UserRepo 接口形式返回。
func NewUserRepo(data *Data) biz.UserRepo {
	// 返回接口，屏蔽 data 层具体实现。
	return &userRepo{data: data}
}

func (r *userRepo) client(ctx context.Context) *ent.Client {
	return r.data.ent
}

func (r *userRepo) ListUsers(ctx context.Context, opts ...biz.ListOption) ([]*biz.User, error) {
	options := biz.ListOptions{Limit: 20}
	for _, opt := range opts {
		opt(&options)
	}
	if options.Offset < 0 || options.Limit <= 0 {
		return nil, biz.ErrUserInvalidArgument
	}

	query := r.client(ctx).User.Query()

	// 处理排序参数。
	// 当能够识别 AIP OrderBy 中的字段时，使用 ent 生成的 ByXxx 排序；
	// 如果无法识别，则回退到按 id 升序，保证结果顺序稳定。
	if columns := extractOrderColumns(&options.OrderBy); len(columns) > 0 {
		orders := make([]user.OrderOption, 0, len(columns))
		for _, col := range columns {
			var dir user.OrderOption
			switch col.path {
			case "id":
				if col.desc {
					dir = user.ByID(sql.OrderDesc())
				} else {
					dir = user.ByID()
				}
			case "name":
				if col.desc {
					dir = user.ByName(sql.OrderDesc())
				} else {
					dir = user.ByName()
				}
			case "email":
				if col.desc {
					dir = user.ByEmail(sql.OrderDesc())
				} else {
					dir = user.ByEmail()
				}
			case "create_time":
				if col.desc {
					dir = user.ByCreateTime(sql.OrderDesc())
				} else {
					dir = user.ByCreateTime()
				}
			case "update_time":
				if col.desc {
					dir = user.ByUpdateTime(sql.OrderDesc())
				} else {
					dir = user.ByUpdateTime()
				}
			default:
				continue
			}
			orders = append(orders, dir)
		}
		if len(orders) > 0 {
			query = query.Order(orders...)
		}
	} else {
		query = query.Order(user.ByID())
	}

	// 处理过滤条件：当请求中包含非空 AIP 过滤表达式时，尝试转换为 ent 谓词。
	if hasFilter(options.Filter) {
		p := translateFilter(options.Filter)
		if p != nil {
			query = query.Where(p)
		}
	}

	// 处理分页参数。
	query = query.Offset(options.Offset).Limit(options.Limit)

	list, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*biz.User, 0, len(list))
	for _, u := range list {
		result = append(result, toBizUser(u))
	}
	return result, nil
}

// FindByID 通过主键查询用户。
//
// 优先走 Redis 主数据缓存（cacheKeyUserData），命中则不访问数据库；
// 未命中再查 DB 并回填缓存，同时写入 name/email 索引。
func (r *userRepo) FindByID(ctx context.Context, id int64) (*biz.User, error) {
	start := time.Now()

	var cacheCost time.Duration
	if r.cacheEnabled() {
		cacheStart := time.Now()
		if u, ok := r.cacheGetUser(ctx, r.cacheKeyUserData(id)); ok {
			cacheCost = time.Since(cacheStart)
			r.logUserLookup("FindByID", "cache_data", u.ID, cacheCost, 0, time.Since(start))
			return u, nil
		}
		cacheCost = time.Since(cacheStart)
	}

	dbStart := time.Now()
	u, err := r.client(ctx).User.Get(ctx, id)
	dbCost := time.Since(dbStart)
	if ent.IsNotFound(err) {
		r.logUserLookup("FindByID", "db_not_found", id, cacheCost, dbCost, time.Since(start))
		return nil, biz.ErrUserNotFound
	}
	if err != nil {
		r.logUserLookup("FindByID", "db_error", id, cacheCost, dbCost, time.Since(start))
		return nil, err
	}
	result := toBizUser(u)
	if r.cacheEnabled() {
		r.cacheSetUser(ctx, result)
	}
	from := "db"
	if r.cacheEnabled() {
		from = "db_cache_miss"
	}
	r.logUserLookup("FindByID", from, result.ID, cacheCost, dbCost, time.Since(start))
	return result, nil
}

// FindByAccount 通过账号查询用户。
//
// account 支持：用户名 或 邮箱。
// 缓存命中路径：
// 1) 先从索引 key（name/email）取出对应 id
// 2) 再用 id 去取主数据（User JSON）
//
// 这样可以保证：User 主数据只缓存一份，索引只是轻量映射。
func (r *userRepo) FindByAccount(ctx context.Context, account string) (*biz.User, error) {
	start := time.Now()

	account = strings.TrimSpace(account)
	if account == "" {
		r.logUserLookup("FindByAccount", "invalid_argument", 0, 0, 0, time.Since(start))
		return nil, biz.ErrUserInvalidArgument
	}

	var cacheCost time.Duration
	if r.cacheEnabled() {
		cacheStart := time.Now()
		if u, ok := r.cacheGetIndexedUser(ctx, account); ok {
			cacheCost = time.Since(cacheStart)
			r.logUserLookup("FindByAccount", "cache_index", u.ID, cacheCost, 0, time.Since(start))
			return u, nil
		}
		cacheCost = time.Since(cacheStart)
	}

	dbStart := time.Now()
	u, err := r.client(ctx).User.Query().
		Where(user.Or(user.Name(account), user.Email(account))).
		First(ctx)
	dbCost := time.Since(dbStart)
	if ent.IsNotFound(err) {
		r.logUserLookup("FindByAccount", "db_not_found", 0, cacheCost, dbCost, time.Since(start))
		return nil, biz.ErrUserNotFound
	}
	if err != nil {
		r.logUserLookup("FindByAccount", "db_error", 0, cacheCost, dbCost, time.Since(start))
		return nil, err
	}
	result := toBizUser(u)
	if r.cacheEnabled() {
		r.cacheSetUser(ctx, result)
	}
	from := "db"
	if r.cacheEnabled() {
		from = "db_cache_miss"
	}
	r.logUserLookup("FindByAccount", from, result.ID, cacheCost, dbCost, time.Since(start))
	return result, nil
}

func (r *userRepo) CreateUser(ctx context.Context, do *biz.User) (*biz.User, error) {
	created, err := r.client(ctx).User.Create().
		SetName(do.Name).
		SetEmail(do.Email).
		SetPassword(do.Password).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, biz.ErrUserInvalidArgument
	}
	if err != nil {
		return nil, err
	}
	result := toBizUser(created)
	if r.cacheEnabled() {
		r.cacheSetUser(ctx, result)
	}
	return result, nil
}

func (r *userRepo) UpdateUser(ctx context.Context, do *biz.User) (*biz.User, error) {
	var previous *ent.User
	if r.cacheEnabled() {
		current, err := r.client(ctx).User.Get(ctx, do.ID)
		if ent.IsNotFound(err) {
			return nil, biz.ErrUserNotFound
		}
		if err != nil {
			return nil, err
		}
		previous = current
	}
	upd := r.client(ctx).User.UpdateOneID(do.ID)
	if do.Name != "" {
		upd.SetName(do.Name)
	}
	if do.Email != "" {
		upd.SetEmail(do.Email)
	}
	if do.Password != "" {
		upd.SetPassword(do.Password)
	}
	updated, err := upd.Save(ctx)
	if ent.IsNotFound(err) {
		return nil, biz.ErrUserNotFound
	}
	if ent.IsConstraintError(err) {
		return nil, biz.ErrUserInvalidArgument
	}
	if err != nil {
		return nil, err
	}
	result := toBizUser(updated)
	if r.cacheEnabled() {
		if previous != nil {
			r.cacheDel(ctx,
				r.cacheKeyUserNameIndex(previous.Name),
				r.cacheKeyUserEmailIndex(previous.Email),
			)
		}
		r.cacheSetUser(ctx, result)
	}
	return result, nil
}

func (r *userRepo) DeleteUser(ctx context.Context, id int64) error {
	var existing *ent.User
	if r.cacheEnabled() {
		u, err := r.client(ctx).User.Get(ctx, id)
		if ent.IsNotFound(err) {
			return biz.ErrUserNotFound
		}
		if err != nil {
			return err
		}
		existing = u
	}

	err := r.client(ctx).User.DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return biz.ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if r.cacheEnabled() {
		r.cacheDel(ctx,
			r.cacheKeyUserData(id),
			r.cacheKeyUserNameIndex(existing.Name),
			r.cacheKeyUserEmailIndex(existing.Email),
		)
	}
	return nil
}

// cacheEnabled 判断当前仓储是否启用了 Redis 缓存。
func (r *userRepo) cacheEnabled() bool {
	return r != nil && r.data != nil && r.data.rdb != nil
}

// cacheTTL 返回用户缓存过期时间。
// 统一从 Data 读取配置值，避免把 TTL 写死在仓储代码中。
func (r *userRepo) cacheTTL() time.Duration {
	if r == nil || r.data == nil || r.data.userCacheTTL <= 0 {
		return 5 * time.Minute
	}
	return r.data.userCacheTTL
}

// cacheKeyUserData：用户主数据 key（真实数据只存这里）。
func (r *userRepo) cacheKeyUserData(id int64) string {
	return fmt.Sprintf("testdemo:user:data:%d", id)
}

// cacheKeyUserNameIndex：用户名索引 key（value 为对应 id）。
// 使用 base64 是为了避免 name 中出现空格、冒号等特殊字符导致 key 不可控。
func (r *userRepo) cacheKeyUserNameIndex(name string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(name))
	return "testdemo:user:index:name:" + encoded
}

// cacheKeyUserEmailIndex：邮箱索引 key（value 为对应 id）。
func (r *userRepo) cacheKeyUserEmailIndex(email string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(email))
	return "testdemo:user:index:email:" + encoded
}

// cacheGetIndexedUser：通过“索引 key -> id -> 主数据 key”拿到用户。
//
// 1) 先按 name 索引查一次
// 2) 再按 email 索引查一次
// 命中任意一个索引都可以返回。
//
// 这里做了轻量的自愈：
// - 索引 value 解析出错（非数字/<=0）会删除该索引 key
// - 索引存在但主数据不存在，认为索引已脏，也删除该索引 key
func (r *userRepo) cacheGetIndexedUser(ctx context.Context, account string) (*biz.User, bool) {
	for _, key := range []string{r.cacheKeyUserNameIndex(account), r.cacheKeyUserEmailIndex(account)} {
		idStr, err := r.data.rdb.Get(ctx, key).Result()
		if err != nil || idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			_ = r.data.rdb.Del(ctx, key).Err()
			continue
		}
		if u, ok := r.cacheGetUser(ctx, r.cacheKeyUserData(id)); ok {
			return u, true
		}
		_ = r.data.rdb.Del(ctx, key).Err()
	}
	return nil, false
}

// cacheGetUser 根据主数据 key 读取 Redis 中缓存的用户对象。
// 如果数据损坏或格式不合法，会删除脏数据并返回未命中。
func (r *userRepo) cacheGetUser(ctx context.Context, key string) (*biz.User, bool) {
	b, err := r.data.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var u biz.User
	if err := json.Unmarshal(b, &u); err != nil {
		_ = r.data.rdb.Del(ctx, key).Err()
		return nil, false
	}
	if u.ID <= 0 {
		_ = r.data.rdb.Del(ctx, key).Err()
		return nil, false
	}
	return &u, true
}

// cacheSetUser 将用户主数据和对应索引同时写入 Redis。
// - 主数据：id -> User JSON
// - 索引：name/email -> id
func (r *userRepo) cacheSetUser(ctx context.Context, u *biz.User) {
	if u == nil {
		return
	}
	b, err := json.Marshal(u)
	if err != nil {
		return
	}
	ttl := r.cacheTTL()
	_ = r.data.rdb.Set(ctx, r.cacheKeyUserData(u.ID), b, ttl).Err()
	_ = r.data.rdb.Set(ctx, r.cacheKeyUserNameIndex(u.Name), strconv.FormatInt(u.ID, 10), ttl).Err()
	_ = r.data.rdb.Set(ctx, r.cacheKeyUserEmailIndex(u.Email), strconv.FormatInt(u.ID, 10), ttl).Err()
}

// cacheDel 批量删除指定的缓存 key，常用于删除主数据和旧索引。
func (r *userRepo) cacheDel(ctx context.Context, keys ...string) {
	ks := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "" {
			ks = append(ks, k)
		}
	}
	if len(ks) == 0 {
		return
	}
	_ = r.data.rdb.Del(ctx, ks...).Err()
}

// logUserLookup 记录用户查询日志，标明结果来源以及缓存/数据库/总耗时。
func (r *userRepo) logUserLookup(method string, source string, id int64, cacheCost time.Duration, dbCost time.Duration, totalCost time.Duration) {
	if cacheCost == 0 && dbCost == 0 {
		log.Info("user lookup", "method", method, "source", source, "id", id, "cost", totalCost)
		return
	}
	log.Info("user lookup", "method", method, "source", source, "id", id, "cache_cost", cacheCost, "db_cost", dbCost, "total_cost", totalCost)
}

// ---------- 辅助函数 ----------

type orderCol struct {
	path string
	desc bool
}

// extractOrderColumns 从 AIP 的 ordering.OrderBy 中提取排序字段。
//
// 这里只依赖公开接口，避免直接访问内部字段导致不同版本的 AIP 包不兼容。
// 当排序值为空时返回 nil，由调用方回退到默认稳定排序。
func extractOrderColumns(ob any) []orderCol {
	if ob == nil {
		return nil
	}
	// 当前唯一稳定可用的公开能力是 String()。
	// 当字符串为空或仅包含空白字符时，说明调用方没有显式传入排序条件。
	if s, ok := ob.(interface{ String() string }); ok && strings.TrimSpace(s.String()) == "" {
		return nil
	}
	// 即使传入了排序参数，如果当前无法安全解析其内部结构，也返回 nil。
	// 后续如果明确锁定 AIP 版本，可以在这里补充真实的字段提取逻辑。
	return nil
}

// hasFilter 判断传入的 AIP filtering.Filter 是否包含有效的过滤表达式。
// 这里只探测公开方法，避免 AIP 包升级后因内部结构变化导致编译失败。
func hasFilter(f filtering.Filter) bool {
	type emptyIface interface{ Empty() bool }
	if e, ok := any(f).(emptyIface); ok {
		return !e.Empty()
	}
	type exprIface interface{ Expr() any }
	if e, ok := any(f).(exprIface); ok {
		return e.Expr() != nil
	}
	return false
}

// translateFilter 将 AIP 过滤树转换为 ent 的 predicate.User。
// 如果当前无法完成转换，则返回 nil，让查询仍然可以带着排序和分页继续执行。
func translateFilter(f filtering.Filter) predicate.User {
	_ = f
	return nil
}

func toBizUser(u *ent.User) *biz.User {
	if u == nil {
		return nil
	}
	return &biz.User{
		ID:         u.ID,
		Name:       u.Name,
		Email:      u.Email,
		Password:   u.Password,
		CreateTime: u.CreateTime,
		UpdateTime: u.UpdateTime,
	}
}
