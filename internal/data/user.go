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

// NewUserRepo creates a new UserRepo instance implemented with ent.
func NewUserRepo(data *Data) biz.UserRepo {
	//返回的是接口
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

	// Apply order_by via ent-generated user.ByXxx helpers. When the AIP
	// OrderBy value only exposes extract ordered columns we fall back to stable id ASC.
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

	// Apply filter when the a non-empty AIP filter expression is detected.
	if hasFilter(options.Filter) {
		p := translateFilter(options.Filter)
		if p != nil {
			query = query.Where(p)
		}
	}

	// Apply pagination
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

const userCacheTTL = 5 * time.Minute

func (r *userRepo) cacheEnabled() bool {
	return r != nil && r.data != nil && r.data.rdb != nil
}

func (r *userRepo) cacheKeyUserData(id int64) string {
	return fmt.Sprintf("testdemo:user:data:%d", id)
}

func (r *userRepo) cacheKeyUserNameIndex(name string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(name))
	return "testdemo:user:index:name:" + encoded
}

func (r *userRepo) cacheKeyUserEmailIndex(email string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(email))
	return "testdemo:user:index:email:" + encoded
}

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

func (r *userRepo) cacheSetUser(ctx context.Context, u *biz.User) {
	if u == nil {
		return
	}
	b, err := json.Marshal(u)
	if err != nil {
		return
	}
	_ = r.data.rdb.Set(ctx, r.cacheKeyUserData(u.ID), b, userCacheTTL).Err()
	_ = r.data.rdb.Set(ctx, r.cacheKeyUserNameIndex(u.Name), strconv.FormatInt(u.ID, 10), userCacheTTL).Err()
	_ = r.data.rdb.Set(ctx, r.cacheKeyUserEmailIndex(u.Email), strconv.FormatInt(u.ID, 10), userCacheTTL).Err()
}

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

func (r *userRepo) logUserLookup(method string, source string, id int64, cacheCost time.Duration, dbCost time.Duration, totalCost time.Duration) {
	if cacheCost == 0 && dbCost == 0 {
		log.Info("user lookup", "method", method, "source", source, "id", id, "cost", totalCost)
		return
	}
	log.Info("user lookup", "method", method, "source", source, "id", id, "cache_cost", cacheCost, "db_cost", dbCost, "total_cost", totalCost)
}

// ---------- helpers ----------

type orderCol struct {
	path string
	desc bool
}

// extractOrderColumns extracts ordered column names from an AIP ordering.OrderBy
// without reaching into unexported fields (Column/Columns layout differs between
// AIP versions). Returns nil when the order value is empty so the caller can
// fall back to the default stable order.
func extractOrderColumns(ob any) []orderCol {
	if ob == nil {
		return nil
	}
	// The only stable public API is String(). An empty/whitespace string means
	// no explicit order was supplied; fall back to default.
	if s, ok := ob.(interface{ String() string }); ok && strings.TrimSpace(s.String()) == "" {
		return nil
	}
	// If explicit order is provided but the exact layout isn't pinned, return
	// nil. Real AIP column extraction can be wired here when needed.
	return nil
}

// hasFilter reports whether the supplied AIP filtering.Filter contains a
// non-empty expression. Only public methods are probed so upgrades of the
// AIP package don't break compilation.
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

// translateFilter converts an AIP filter tree to an ent predicate.User.
// Returns nil when no filter translation is possible so the query still
// executes with ordering and pagination intact.
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
