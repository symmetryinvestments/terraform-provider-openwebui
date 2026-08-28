package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	groupPermissionsWorkspaceKeys = []string{"models", "knowledge", "prompts", "tools", "skills", "skills_import", "skills_export", "models_import", "models_export", "prompts_import", "prompts_export", "tools_import", "tools_export"}
	groupPermissionsSharingKeys   = []string{"models", "knowledge", "prompts", "tools", "skills", "notes", "public_models", "public_knowledge", "public_prompts", "public_tools", "public_skills", "public_notes", "public_chats", "folders", "open_chats", "public_calendars"}
	groupPermissionsAccessGrantsKeys = []string{"allow_users", "allow_groups"}
	groupPermissionsChatKeys      = []string{"controls", "valves", "system_prompt", "params", "file_upload", "web_upload", "delete", "delete_message", "continue_response", "regenerate_response", "rate_response", "edit", "share", "export", "stt", "tts", "call", "multiple_models", "temporary", "temporary_enforced", "import"}
	groupPermissionsFeaturesKeys  = []string{"api_keys", "notes", "channels", "folders", "direct_tool_servers", "web_search", "image_generation", "code_interpreter", "memories", "automations", "calendar", "webhooks"}
	groupPermissionsSettingsKeys  = []string{"interface"}

	groupPermissionsAllowedSets = map[string]map[string]struct{}{
		"workspace": sliceToSet(groupPermissionsWorkspaceKeys),
		"sharing":   sliceToSet(groupPermissionsSharingKeys),
		"access_grants": sliceToSet(groupPermissionsAccessGrantsKeys),
		"chat":      sliceToSet(groupPermissionsChatKeys),
		"features":  sliceToSet(groupPermissionsFeaturesKeys),
		"settings":  sliceToSet(groupPermissionsSettingsKeys),
	}
)

func permissionsSpecified(perms *groupPermissionsModel) bool {
	return mapProvided(perms.Workspace) || mapProvided(perms.Sharing) || mapProvided(perms.AccessGrants) || mapProvided(perms.Chat) || mapProvided(perms.Features) || mapProvided(perms.Settings)
}

func mapProvided(value types.Map) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func sliceToSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}

	return set
}

func expandPermissions(ctx context.Context, perms *groupPermissionsModel, diags *diag.Diagnostics) map[string]any {
	result := make(map[string]any)

	add := func(category string, value types.Map, attribute path.Path) {
		if value.IsNull() || value.IsUnknown() {
			return
		}

		var bools map[string]bool
		if err := value.ElementsAs(ctx, &bools, false); err != nil {
			diags.AddAttributeError(
				attribute,
				"Invalid permissions value",
				fmt.Sprintf("Unable to decode %s into a map of booleans: %v", attribute.String(), err),
			)
			return
		}

		filtered := filterPermissionKeys(category, bools, attribute, diags)
		if len(filtered) == 0 {
			return
		}

		nested := make(map[string]any, len(filtered))
		for k, v := range filtered {
			nested[k] = v
		}

		result[category] = nested
	}

	add("workspace", perms.Workspace, path.Root("permissions").AtName("workspace"))
	add("sharing", perms.Sharing, path.Root("permissions").AtName("sharing"))
	add("access_grants", perms.AccessGrants, path.Root("permissions").AtName("access_grants"))
	add("chat", perms.Chat, path.Root("permissions").AtName("chat"))
	add("features", perms.Features, path.Root("permissions").AtName("features"))
	add("settings", perms.Settings, path.Root("permissions").AtName("settings"))

	if len(result) == 0 {
		return nil
	}

	return result
}

func flattenPermissions(ctx context.Context, perms map[string]any) (*groupPermissionsModel, diag.Diagnostics) {
	model := &groupPermissionsModel{
		Workspace: types.MapNull(types.BoolType),
		Sharing:   types.MapNull(types.BoolType),
		AccessGrants: types.MapNull(types.BoolType),
		Chat:      types.MapNull(types.BoolType),
		Features:  types.MapNull(types.BoolType),
		Settings:  types.MapNull(types.BoolType),
	}

	var diags diag.Diagnostics

	if len(perms) == 0 {
		return model, diags
	}

	convert := func(category string) types.Map {
		raw, ok := perms[category]
		if !ok || raw == nil {
			return types.MapNull(types.BoolType)
		}

		nested, ok := raw.(map[string]any)
		if !ok {
			diags.AddError(
				"Unexpected permissions response",
				fmt.Sprintf("Expected permissions.%s to be an object", category),
			)
			return types.MapNull(types.BoolType)
		}

		bools := filterPermissionResponse(category, nested, &diags)
		tfMap, mapDiags := types.MapValueFrom(ctx, types.BoolType, bools)
		diags.Append(mapDiags...)
		return tfMap
	}

	model.Workspace = convert("workspace")
	model.Sharing = convert("sharing")
	model.AccessGrants = convert("access_grants")
	model.Chat = convert("chat")
	model.Features = convert("features")
	model.Settings = convert("settings")

	return model, diags
}

func filterPermissionKeys(category string, bools map[string]bool, attribute path.Path, diags *diag.Diagnostics) map[string]bool {
	allowed, ok := groupPermissionsAllowedSets[category]
	if !ok {
		diags.AddError(
			"Internal provider error",
			fmt.Sprintf("Unknown permission category %s", category),
		)
		return nil
	}

	allowedList := allowedKeysList(category)

	filtered := make(map[string]bool, len(bools))
	for key, value := range bools {
		if _, exists := allowed[key]; !exists {
			diags.AddAttributeError(
				attribute,
				fmt.Sprintf("Unsupported %s permission key", category),
				fmt.Sprintf("Supported keys are: %s. Received %q.", allowedList, key),
			)
			continue
		}

		filtered[key] = value
	}

	return filtered
}

func filterPermissionResponse(category string, nested map[string]any, diags *diag.Diagnostics) map[string]bool {
	allowed, ok := groupPermissionsAllowedSets[category]
	if !ok {
		diags.AddError(
			"Internal provider error",
			fmt.Sprintf("Unknown permission category %s", category),
		)
		return nil
	}

	allowedList := allowedKeysList(category)

	filtered := make(map[string]bool, len(nested))
	for key, raw := range nested {
		boolVal, ok := raw.(bool)
		if !ok {
			diags.AddError(
				"Unexpected permissions response",
				fmt.Sprintf("Expected permissions.%s.%s to be a boolean", category, key),
			)
			continue
		}

		if _, exists := allowed[key]; !exists {
			diags.AddError(
				"Unexpected permissions response",
				fmt.Sprintf("Encountered unsupported %s permission %q from API. Supported keys: %s.", category, key, allowedList),
			)
			continue
		}

		filtered[key] = boolVal
	}

	return filtered
}

func allowedKeysList(category string) string {
	var keys []string

	switch category {
	case "workspace":
		keys = groupPermissionsWorkspaceKeys
	case "sharing":
		keys = groupPermissionsSharingKeys
	case "access_grants":
		keys = groupPermissionsAccessGrantsKeys
	case "chat":
		keys = groupPermissionsChatKeys
	case "features":
		keys = groupPermissionsFeaturesKeys
	case "settings":
		keys = groupPermissionsSettingsKeys
	default:
		return ""
	}

	return strings.Join(keys, ", ")
}
