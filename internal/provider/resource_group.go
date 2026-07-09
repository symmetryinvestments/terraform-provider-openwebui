package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/nickcecere/terraform-provider-openwebui/internal/client"
)

var _ resource.Resource = &groupResource{}
var _ resource.ResourceWithConfigure = &groupResource{}
var _ resource.ResourceWithImportState = &groupResource{}
var _ resource.ResourceWithValidateConfig = &groupResource{}

// groupResource manages Open WebUI groups.
type groupResource struct {
	client *client.Client
}

// groupBaseModel holds the fields common to both the resource and the data source.
type groupBaseModel struct {
	ID          types.String           `tfsdk:"id"`
	Name        types.String           `tfsdk:"name"`
	Description types.String           `tfsdk:"description"`
	Users       types.Set              `tfsdk:"users"`
	Permissions *groupPermissionsModel `tfsdk:"permissions"`
	UserID      types.String           `tfsdk:"user_id"`
	CreatedAt   types.String           `tfsdk:"created_at"`
	UpdatedAt   types.String           `tfsdk:"updated_at"`
}

// groupResourceModel maps Terraform resource state.
type groupResourceModel struct {
	groupBaseModel
	ManageUsers types.Bool `tfsdk:"manage_users"`
}

type groupPermissionsModel struct {
	Workspace types.Map `tfsdk:"workspace"`
	Sharing   types.Map `tfsdk:"sharing"`
	AccessGrants types.Map `tfsdk:"access_grants"`
	Chat      types.Map `tfsdk:"chat"`
	Features  types.Map `tfsdk:"features"`
	Settings types.Map `tfsdk:"settings"`
}

// NewGroupResource constructs a new resource instance.
func NewGroupResource() resource.Resource {
	return &groupResource{}
}

// Metadata implements resource.Resource.
func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

// Schema defines the resource schema for groups.
func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Unique identifier assigned by Open WebUI.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Group name.",
			},
			"description": schema.StringAttribute{
				Required:    true,
				Description: "Group description.",
			},
			"users": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Usernames or email addresses resolved to user IDs when managing group membership. Must not be set when manage_users is false.",
			},
			"manage_users": schema.BoolAttribute{
				Optional:    true,
				Description: "When false, group membership is managed externally. The provider will not add or remove users, and external membership changes will not cause plan differences. If missing, treated as true.",
			},
			"permissions": schema.SingleNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Fine-grained permission flags organised by category.",
				Attributes: map[string]schema.Attribute{
					"workspace": schema.MapAttribute{
						Optional:      true,
						Computed:      true,
						ElementType:   types.BoolType,
						Description:   "Workspace-level permissions.",
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
						Validators: []validator.Map{
							mapvalidator.KeysAre(stringvalidator.OneOf(groupPermissionsWorkspaceKeys...)),
						},
					},
					"sharing": schema.MapAttribute{
						Optional:      true,
						Computed:      true,
						ElementType:   types.BoolType,
						Description:   "Sharing permissions.",
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
						Validators: []validator.Map{
							mapvalidator.KeysAre(stringvalidator.OneOf(groupPermissionsSharingKeys...)),
						},
					},
					"access_grants": schema.MapAttribute{
						Optional:      true,
						Computed:      true,
						ElementType:   types.BoolType,
						Description:   "Access grants permissions.",
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
						Validators: []validator.Map{
							mapvalidator.KeysAre(stringvalidator.OneOf(groupPermissionsAccessGrantsKeys...)),
						},
					},
					"chat": schema.MapAttribute{
						Optional:      true,
						Computed:      true,
						ElementType:   types.BoolType,
						Description:   "Chat-related permissions.",
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
						Validators: []validator.Map{
							mapvalidator.KeysAre(stringvalidator.OneOf(groupPermissionsChatKeys...)),
						},
					},
					"features": schema.MapAttribute{
						Optional:      true,
						Computed:      true,
						ElementType:   types.BoolType,
						Description:   "Feature toggle permissions.",
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
						Validators: []validator.Map{
							mapvalidator.KeysAre(stringvalidator.OneOf(groupPermissionsFeaturesKeys...)),
						},
					},
					"settings": schema.MapAttribute{
						Optional:      true,
						Computed:      true,
						ElementType:   types.BoolType,
						Description:   "Settings permissions.",
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
						Validators: []validator.Map{
							mapvalidator.KeysAre(stringvalidator.OneOf(groupPermissionsSettingsKeys...)),
						},
					},
				},
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the user who owns the group.",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation date assigned by Open WebUI (YYYY-MM-DD).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Last update date assigned by Open WebUI (YYYY-MM-DD).",
			},
		},
	}
}

// Configure connects the API client to the resource.
func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	if client, ok := req.ProviderData.(*client.Client); ok {
		r.client = client
	}
}

// ValidateConfig enforces cross-attribute constraints at plan time.
func (r *groupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config groupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.ManageUsers.IsNull() && !config.ManageUsers.ValueBool() && !config.Users.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("users"),
			"Conflicting configuration",
			"users cannot be set when manage_users is false.",
		)
	}
}

// Create provisions a group.
func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Unconfigured API client", "Expected provider to configure the Open WebUI client before managing groups.")
		return
	}

	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managingUsers := plan.ManageUsers.IsNull() || plan.ManageUsers.ValueBool()

	form := client.GroupForm{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	created, err := r.client.CreateGroup(ctx, form)
	if err != nil {
		resp.Diagnostics.AddError("Create group failed", err.Error())
		return
	}

	updateForm := client.GroupUpdateForm{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	providedPermissions := permissionsSpecified(plan.Permissions)
	providedMeta := false
	providedData := false

	if managingUsers && !plan.Users.IsNull() && !plan.Users.IsUnknown() {
		usernames := expandStringSet(ctx, plan.Users, path.Root("users"), &resp.Diagnostics)
		resolvedUserIDs := uniqueStrings(resolveUsernamesToIDs(ctx, r.client, usernames, path.Root("users"), &resp.Diagnostics))
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.AddGroupUsers(ctx, created.ID, resolvedUserIDs); err != nil {
			resp.Diagnostics.AddError("Add group members failed", err.Error())
			return
		}
	}

	updateForm.Permissions = expandPermissions(ctx, plan.Permissions, &resp.Diagnostics)
	updateForm.Meta = nil
	updateForm.Data = nil

	if resp.Diagnostics.HasError() {
		return
	}

	if providedPermissions || providedMeta || providedData {
		if _, err := r.client.UpdateGroup(ctx, created.ID, updateForm); err != nil {
			resp.Diagnostics.AddError("Update group failed", err.Error())
			return
		}
	}

	current, err := r.client.GetGroup(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}

	base, diags := groupResponseToModel(ctx, r.client, current, managingUsers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &groupResourceModel{groupBaseModel: base, ManageUsers: plan.ManageUsers})...)
}

// Read refreshes state from the API.
func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Unconfigured API client", "Expected provider to configure the Open WebUI client before managing groups.")
		return
	}

	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetGroup(ctx, state.ID.ValueString())
	if err != nil {
		if err == client.ErrNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}

	fetchUsers := state.ManageUsers.IsNull() || state.ManageUsers.ValueBool()
	base, diags := groupResponseToModel(ctx, r.client, current, fetchUsers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &groupResourceModel{groupBaseModel: base, ManageUsers: state.ManageUsers})...)
}

// Update mutates group properties.
func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Unconfigured API client", "Expected provider to configure the Open WebUI client before managing groups.")
		return
	}

	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managingUsers := plan.ManageUsers.IsNull() || plan.ManageUsers.ValueBool()

	form := client.GroupUpdateForm{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}
	form.Permissions = expandPermissions(ctx, plan.Permissions, &resp.Diagnostics)
	form.Meta = nil
	form.Data = nil

	if managingUsers {
		currentUsers, err := r.client.GetGroupUsers(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Read group users failed", err.Error())
			return
		}

		usernames := expandStringSet(ctx, plan.Users, path.Root("users"), &resp.Diagnostics)
		desiredIDs := uniqueStrings(resolveUsernamesToIDs(ctx, r.client, usernames, path.Root("users"), &resp.Diagnostics))

		if resp.Diagnostics.HasError() {
			return
		}

		toAdd, toRemove := diffStringSets(extractUserIDs(currentUsers), desiredIDs)

		if err := r.client.RemoveGroupUsers(ctx, plan.ID.ValueString(), toRemove); err != nil {
			resp.Diagnostics.AddError("Remove group members failed", err.Error())
			return
		}

		if err := r.client.AddGroupUsers(ctx, plan.ID.ValueString(), toAdd); err != nil {
			resp.Diagnostics.AddError("Add group members failed", err.Error())
			return
		}
	}

	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateGroup(ctx, plan.ID.ValueString(), form); err != nil {
		resp.Diagnostics.AddError("Update group failed", err.Error())
		return
	}

	fresh, err := r.client.GetGroup(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read group failed", err.Error())
		return
	}

	base, diags := groupResponseToModel(ctx, r.client, fresh, managingUsers)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &groupResourceModel{groupBaseModel: base, ManageUsers: plan.ManageUsers})...)
}

// Delete removes the group.
func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Unconfigured API client", "Expected provider to configure the Open WebUI client before managing groups.")
		return
	}

	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteGroup(ctx, state.ID.ValueString()); err != nil {
		if err == client.ErrNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Delete group failed", err.Error())
		return
	}
}

// ImportState passes the import identifier through to the id attribute.
func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// groupResponseToModel converts an API response to the base model shared by the resource and data source.
// fetchUsers controls whether group membership is retrieved from the API; pass false when
// membership is managed externally so no API call is made and users is left null.
func groupResponseToModel(ctx context.Context, apiClient *client.Client, resp *client.GroupResponse, fetchUsers bool) (groupBaseModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	permissions, permDiags := flattenPermissions(ctx, resp.Permissions)
	diags.Append(permDiags...)

	var usersSet types.Set
	if fetchUsers {
		users, err := apiClient.GetGroupUsers(ctx, resp.ID)
		if err != nil {
			diags.AddError("Read group users failed", err.Error())
		}
		usernames := extractUserLabels(users)
		var usersDiags diag.Diagnostics
		usersSet, usersDiags = types.SetValueFrom(ctx, types.StringType, usernames)
		diags.Append(usersDiags...)
	} else {
		usersSet = types.SetNull(types.StringType)
	}

	model := groupBaseModel{
		ID:          types.StringValue(resp.ID),
		Name:        types.StringValue(resp.Name),
		Description: types.StringValue(resp.Description),
		Users:       usersSet,
		Permissions: permissions,
		UserID:      types.StringValue(resp.UserID),
		CreatedAt:   formatDateValue(resp.CreatedAt),
		UpdatedAt:   formatDateValue(resp.UpdatedAt),
	}

	return model, diags
}

func resolveUsernamesToIDs(ctx context.Context, apiClient *client.Client, identifiers []string, attribute path.Path, diags *diag.Diagnostics) []string {
	if len(identifiers) == 0 {
		return nil
	}

	var ids []string
	for _, identifier := range identifiers {
		id, err := lookupUserID(ctx, apiClient, identifier)
		if err != nil {
			diags.AddAttributeError(
				attribute,
				"Unable to resolve user identifier",
				fmt.Sprintf("Failed to map %q to an Open WebUI user ID: %v", identifier, err),
			)
			continue
		}
		ids = append(ids, id)
	}

	return ids
}

func lookupUserID(ctx context.Context, apiClient *client.Client, identifier string) (string, error) {
	users, _, err := apiClient.SearchUsers(ctx, identifier, 1)
	if err != nil {
		return "", err
	}

	if len(users) == 0 {
		return "", fmt.Errorf("no matching users found")
	}

	// Prefer exact matches on email, username, or name.
	for _, u := range users {
		if strings.EqualFold(u.Email, identifier) {
			return u.ID, nil
		}
		if u.Username != nil && strings.EqualFold(*u.Username, identifier) {
			return u.ID, nil
		}
		if strings.EqualFold(u.Name, identifier) {
			return u.ID, nil
		}
	}

	if len(users) == 1 {
		return users[0].ID, nil
	}

	normalized := strings.ToLower(identifier)
	for _, u := range users {
		if strings.Contains(strings.ToLower(u.Email), normalized) {
			return u.ID, nil
		}
		if u.Username != nil && strings.Contains(strings.ToLower(*u.Username), normalized) {
			return u.ID, nil
		}
	}

	return users[0].ID, nil
}

func extractUserLabels(users []client.User) []string {
	var names []string
	for _, user := range users {
		label := user.Email
		if label == "" {
			if user.Username != nil && *user.Username != "" {
				label = *user.Username
			} else {
				label = user.Name
			}
		}

		if label == "" {
			label = user.ID
		}

		names = append(names, label)
	}

	sort.Strings(names)
	return names
}

func extractUserIDs(users []client.User) []string {
	var ids []string
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	return ids
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}

	seen := make(map[string]struct{}, len(values))
	var result []string
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}

	return result
}

func diffStringSets(current, desired []string) (toAdd, toRemove []string) {
	currentSet := make(map[string]struct{}, len(current))
	desiredSet := make(map[string]struct{}, len(desired))

	for _, id := range current {
		currentSet[id] = struct{}{}
	}
	for _, id := range desired {
		desiredSet[id] = struct{}{}
		if _, ok := currentSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}

	for id := range currentSet {
		if _, ok := desiredSet[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}

	return uniqueStrings(toAdd), uniqueStrings(toRemove)
}
