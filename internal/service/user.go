package service

import (
	"context"

	v1 "testdemo/api/todo/v1"
	"testdemo/internal/biz"

	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	"go.einride.tech/aip/pagination"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserService is a user service.
type UserService struct {
	v1.UnimplementedUserServiceServer
	uc *biz.UserUsecase
}

// NewUserService new a user service.
func NewUserService(uc *biz.UserUsecase) *UserService {
	return &UserService{uc: uc}
}

// CreateUser creates a new user.
func (s *UserService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.User, error) {
	//自定义转换函数convertUser，转换成biz层所需要的user
	//GetUser安全获取User
	u, err := s.uc.CreateUser(ctx, convertUser(req.GetUser()))
	if err != nil {
		return nil, err
	}
	//convertUserReply自定义返回函数返回proto定义User
	return convertUserReply(u), nil
}

// GetUser returns a user by id.
func (s *UserService) GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.User, error) {
	u, err := s.uc.GetUser(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return convertUserReply(u), nil
}

// ListUsers lists users.
func (s *UserService) ListUsers(ctx context.Context, req *v1.ListUsersRequest) (*v1.UserSet, error) {
	declarations, err := filtering.NewDeclarations(
		filtering.DeclareStandardFunctions(),
		filtering.DeclareIdent("id", filtering.TypeInt),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("email", filtering.TypeString),
		filtering.DeclareIdent("create_time", filtering.TypeTimestamp),
		filtering.DeclareIdent("update_time", filtering.TypeTimestamp),
	)
	if err != nil {
		return nil, err
	}
	filter, err := filtering.ParseFilter(req, declarations)
	if err != nil {
		return nil, err
	}
	pageToken, err := pagination.ParsePageToken(req)
	if err != nil {
		return nil, err
	}
	orderBy, err := ordering.ParseOrderBy(req)
	if err != nil {
		return nil, err
	}
	if err := orderBy.ValidateForPaths("id", "name", "email", "create_time", "update_time"); err != nil {
		return nil, err
	}
	if req.PageSize <= 0 {
		req.PageSize = defaultPageSize
	}
	users, err := s.uc.ListUsers(ctx,
		biz.ListFilter(filter),
		biz.ListOrderBy(orderBy),
		biz.ListLimit(int(req.PageSize)),
		biz.ListOffset(int(pageToken.Offset)),
	)
	if err != nil {
		return nil, err
	}
	set := &v1.UserSet{
		Users: make([]*v1.User, 0, len(users)),
	}
	if len(users) >= int(req.PageSize) {
		set.NextPageToken = pageToken.Next(req).String()
	}
	for _, u := range users {
		set.Users = append(set.Users, convertUserReply(u))
	}
	return set, nil
}

// UpdateUser partially updates a user using a field mask.
func (s *UserService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.User, error) {
	//ID不为0，更新字段不为空，否则提示400
	if req.GetUser().GetId() <= 0 || req.GetUpdateMask() == nil || len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, biz.ErrUserInvalidArgument
	}
	//根据ID操作表
	current, err := s.GetUser(ctx, &v1.GetUserRequest{Id: req.GetUser().GetId()})
	if err != nil {
		return nil, err
	}
	// Don't overwrite password when merging via fieldmask for fields that are INPUT_ONLY.
	fieldmask.Update(req.GetUpdateMask(), current, req.GetUser())
	u, err := s.uc.UpdateUser(ctx, convertUser(current))
	if err != nil {
		return nil, err
	}
	return convertUserReply(u), nil
}

// DeleteUser removes a user by id.
func (s *UserService) DeleteUser(ctx context.Context, req *v1.DeleteUserRequest) (*v1.DeleteUserReply, error) {
	if err := s.uc.DeleteUser(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &v1.DeleteUserReply{
		Code: 200,
		Info: "User deleted",
	}, nil
}

func convertUser(in *v1.User) *biz.User {
	if in == nil {
		return nil
	}
	return &biz.User{
		ID:       in.GetId(),
		Name:     in.GetName(),
		Email:    in.GetEmail(),
		Password: in.GetPassword(),
	}
}

func convertUserReply(in *biz.User) *v1.User {
	if in == nil {
		return nil
	}
	return &v1.User{
		Id:         in.ID,
		Name:       in.Name,
		Email:      in.Email,
		CreateTime: timestamppb.New(in.CreateTime),
		UpdateTime: timestamppb.New(in.UpdateTime),
	}
}
