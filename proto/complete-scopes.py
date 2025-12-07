#!/usr/bin/env python3
"""
Comprehensive script to add OAuth scopes to ALL RPC methods.
"""

import re
from pathlib import Path

# Map RPC method patterns to (resource, level, oauth_scopes)
METHOD_SCOPES = {
    # Firewall - Project level
    'ListProjectFirewallRules': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_READ', ['read:firewall']),
    'CreateProjectFirewallRule': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_WRITE', ['write:firewall']),
    'DeleteProjectFirewallRule': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['delete:firewall']),

    # Firewall - Site level
    'ListSiteFirewallRules': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_READ', ['read:firewall']),
    'CreateSiteFirewallRule': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_WRITE', ['write:firewall']),
    'DeleteSiteFirewallRule': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['delete:firewall']),

    # Members - Organization level
    'ListOrganizationMembers': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_READ', ['read:members']),
    'CreateOrganizationMember': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_WRITE', ['write:members']),
    'UpdateOrganizationMember': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_WRITE', ['write:members']),
    'DeleteOrganizationMember': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN', ['delete:members']),

    # Members - Project level
    'ListProjectMembers': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_READ', ['read:members']),
    'CreateProjectMember': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_WRITE', ['write:members']),
    'UpdateProjectMember': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_WRITE', ['write:members']),
    'DeleteProjectMember': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['delete:members']),

    # Members - Site level
    'ListSiteMembers': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_READ', ['read:members']),
    'CreateSiteMember': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_WRITE', ['write:members']),
    'UpdateSiteMember': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_WRITE', ['write:members']),
    'DeleteSiteMember': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['delete:members']),

    # SSH Keys - Account level
    'GetSshKey': ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_READ', ['read:user']),
    'ListSshKeys': ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_READ', ['read:user']),
    'CreateSshKey': ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_WRITE', ['write:user']),
    'UpdateSshKey': ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_WRITE', ['write:user']),
    'DeleteSshKey': ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_WRITE', ['write:user']),

    # Site Operations
    'GetSiteStatus': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_READ', ['read:site']),
    'DeploySite': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_WRITE', ['write:site']),

    # Secrets - Organization level
    'ListOrganizationSecrets': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'GetOrganizationSecret': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'CreateOrganizationSecret': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'UpdateOrganizationSecret': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'DeleteOrganizationSecret': ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),

    # Secrets - Project level
    'ListProjectSecrets': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'GetProjectSecret': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'CreateProjectSecret': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'UpdateProjectSecret': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'DeleteProjectSecret': ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),

    # Secrets - Site level
    'ListSiteSecrets': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'GetSiteSecret': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'CreateSiteSecret': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'UpdateSiteSecret': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),
    'DeleteSiteSecret': ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN', ['manage_secrets']),

    # Account operations
    'GetAccountByEmail': ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_READ', ['read:user']),
}


def add_scope_to_rpc(content: str, method_name: str, resource: str, level: str, oauth_scopes: list) -> str:
    """Add required_scope to an RPC method that doesn't have it."""

    # Pattern to find the RPC method block
    pattern = rf'(  rpc {method_name}\([^)]+\) returns \([^)]+\) \{{)(.*?)(^\  \}})'

    def replace_func(match):
        rpc_start = match.group(1)
        rpc_body = match.group(2)
        rpc_end = match.group(3)

        # Check if already has required_scope
        if 'required_scope' in rpc_body:
            return match.group(0)

        # Build scope annotation
        oauth_lines = '\n'.join(f'      oauth_scopes: "{scope}"' for scope in oauth_scopes)
        scope_annotation = f'''    option (libops.v1.options.required_scope) = {{
      resource: {resource}
      level: {level}
      allow_parent_access: true
{oauth_lines}
    }};
'''

        return f"{rpc_start}{rpc_body}{scope_annotation}{rpc_end}"

    return re.sub(pattern, replace_func, content, flags=re.DOTALL | re.MULTILINE)


def process_file(filepath: Path):
    """Process a single proto file."""
    with open(filepath, 'r') as f:
        content = f.read()

    original_content = content
    changes = 0

    # Find which methods are in this file
    for method_name, (resource, level, oauth_scopes) in METHOD_SCOPES.items():
        if f'rpc {method_name}' in content:
            new_content = add_scope_to_rpc(content, method_name, resource, level, oauth_scopes)
            if new_content != content:
                content = new_content
                changes += 1
                print(f"  + Added scope to {method_name}")

    if content != original_content:
        with open(filepath, 'w') as f:
            f.write(content)
        return changes

    return 0


def main():
    proto_dir = Path(__file__).parent / "libops" / "v1"

    files = [
        proto_dir / "organization_api.proto",
        proto_dir / "secrets.proto",
        proto_dir / "organization_account_api.proto",
    ]

    total_changes = 0
    for filepath in files:
        if not filepath.exists():
            continue

        print(f"\nProcessing {filepath.name}...")
        changes = process_file(filepath)
        if changes > 0:
            print(f"  ✓ Made {changes} changes")
            total_changes += changes
        else:
            print(f"  - No changes needed")

    print(f"\n✓ Total: {total_changes} scopes added")


if __name__ == '__main__':
    main()
