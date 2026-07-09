package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nickcecere/terraform-provider-openwebui/internal/client"
)

var _ datasource.DataSource = &usersDataSource{}
var _ datasource.DataSourceWithConfigure = &usersDataSource{}

// usersDataSource lists users.
type usersDataSource struct {
	client *client.Client
}

type userListItemModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Email           types.String `tfsdk:"email"`
	Username        types.String `tfsdk:"username"`
	Role            types.String `tfsdk:"role"`
	ProfileImageURL types.String `tfsdk:"profile_image_url"`
	Bio             types.String `tfsdk:"bio"`
	LastActiveAt    types.Int64  `tfsdk:"last_active_at"`
	UpdatedAt       types.Int64  `tfsdk:"updated_at"`
	CreatedAt       types.Int64  `tfsdk:"created_at"`
}

type usersDataSourceModel struct {
	Query types.String        `tfsdk:"query"`
	Users []userListItemModel `tfsdk:"users"`
}

// NewUsersDataSource constructs a new users data source.
func NewUsersDataSource() datasource.DataSource {
	return &usersDataSource{}
}

// Metadata sets the data source type name.
func (d *usersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

// Schema defines the users data source schema.
func (d *usersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"query": schema.StringAttribute{
				Optional:    true,
				Description: "Search query used to filter users by username, email, or name. If omitted, all users are returned.",
			},
			"users": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of users matching the query.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{Computed: true},
						"name":              schema.StringAttribute{Computed: true},
						"email":             schema.StringAttribute{Computed: true},
						"username":          schema.StringAttribute{Computed: true},
						"role":              schema.StringAttribute{Computed: true},
						"profile_image_url": schema.StringAttribute{Computed: true},
						"bio":               schema.StringAttribute{Computed: true},
						"last_active_at":    schema.Int64Attribute{Computed: true},
						"updated_at":        schema.Int64Attribute{Computed: true},
						"created_at":        schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

// Configure assigns the API client.
func (d *usersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	if client, ok := req.ProviderData.(*client.Client); ok {
		d.client = client
	}
}

// Read lists users, paging through the server's results until all matching
// users have been retrieved.
func (d *usersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Unconfigured API client", "Expected provider to configure the Open WebUI client before using the users data source.")
		return
	}

	var config usersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query := ""
	if !config.Query.IsNull() && !config.Query.IsUnknown() {
		query = strings.TrimSpace(config.Query.ValueString())
	}

	page := 1
	users, total, err := d.client.SearchUsers(ctx, query, page)
	if err != nil {
		resp.Diagnostics.AddError("List users failed", err.Error())
		return
	}

	allUsers := append([]client.User{}, users...)
	pageSize := len(users)

	for pageSize > 0 && page*pageSize < total {
		page++
		users, _, err = d.client.SearchUsers(ctx, query, page)
		if err != nil {
			resp.Diagnostics.AddError("List users failed", err.Error())
			return
		}
		allUsers = append(allUsers, users...)
	}

	items := make([]userListItemModel, 0, len(allUsers))
	for _, user := range allUsers {
		username := types.StringNull()
		if user.Username != nil {
			username = types.StringValue(*user.Username)
		}
		profileImage := types.StringNull()
		if user.ProfileImage != "" {
			profileImage = types.StringValue(user.ProfileImage)
		}
		bio := types.StringNull()
		if user.Bio != nil {
			bio = types.StringValue(*user.Bio)
		}

		items = append(items, userListItemModel{
			ID:              types.StringValue(user.ID),
			Name:            types.StringValue(user.Name),
			Email:           types.StringValue(user.Email),
			Username:        username,
			Role:            types.StringValue(user.Role),
			ProfileImageURL: profileImage,
			Bio:             bio,
			LastActiveAt:    types.Int64Value(user.LastActiveAt),
			UpdatedAt:       types.Int64Value(user.UpdatedAt),
			CreatedAt:       types.Int64Value(user.CreatedAt),
		})
	}

	state := usersDataSourceModel{
		Query: config.Query,
		Users: items,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
