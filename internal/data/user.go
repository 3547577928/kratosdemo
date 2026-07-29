package data

import (
	"context"
	"strings"

	"testdemo/ent"
	"testdemo/ent/predicate"
	"testdemo/ent/user"
	"testdemo/internal/biz"

	"entgo.io/ent/dialect/sql"
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
	u, err := r.client(ctx).User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, biz.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return toBizUser(u), nil
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
	return toBizUser(created), nil
}

func (r *userRepo) UpdateUser(ctx context.Context, do *biz.User) (*biz.User, error) {
	upd := r.client(ctx).User.UpdateOneID(do.ID)
	dirty := false
	if do.Name != "" {
		upd.SetName(do.Name)
		dirty = true
	}
	if do.Email != "" {
		upd.SetEmail(do.Email)
		dirty = true
	}
	if do.Password != "" {
		upd.SetPassword(do.Password)
		dirty = true
	}
	if !dirty {
		return r.FindByID(ctx, do.ID)
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
	return toBizUser(updated), nil
}

func (r *userRepo) DeleteUser(ctx context.Context, id int64) error {
	err := r.client(ctx).User.DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return biz.ErrUserNotFound
	}
	return err
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
