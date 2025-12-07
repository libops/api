#!/usr/bin/env python3
"""
Script to inject OAuth scope requirements into OpenAPI spec based on proto scope annotations.

This script:
1. Reads proto files to extract scope annotations
2. Maps proto scope annotations to OAuth 2.0 scope strings
3. Injects security requirements into the OpenAPI spec
4. Adds OAuth2 security scheme definitions
"""

import yaml
import re
import sys
from pathlib import Path
from typing import Dict, List, Set, Optional, Tuple

# Mapping from proto scope resource+level to OAuth scope strings
# This follows the OAuth scopes documented in SCOPES.md
SCOPE_MAPPING = {
    # Account scopes
    ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_READ'): ['read:user', 'read:organizations'],
    ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_WRITE'): ['write:user'],
    ('RESOURCE_TYPE_ACCOUNT', 'ACCESS_LEVEL_ADMIN'): ['write:user'],

    # Organization scopes
    ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_READ'): ['read:organization'],
    ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_WRITE'): ['write:organization', 'read:organization'],
    ('RESOURCE_TYPE_ORGANIZATION', 'ACCESS_LEVEL_ADMIN'): [
        'delete:organization', 'write:organization', 'read:organization', 'manage_secrets'
    ],

    # Project scopes
    ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_READ'): ['read:project'],
    ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_WRITE'): ['write:project', 'read:project'],
    ('RESOURCE_TYPE_PROJECT', 'ACCESS_LEVEL_ADMIN'): [
        'delete:project', 'write:project', 'read:project', 'manage_secrets'
    ],

    # Site scopes
    ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_READ'): ['read:site'],
    ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_WRITE'): ['write:site', 'read:site'],
    ('RESOURCE_TYPE_SITE', 'ACCESS_LEVEL_ADMIN'): [
        'delete:site', 'write:site', 'read:site', 'manage_secrets'
    ],
}

# OAuth scope descriptions for documentation
SCOPE_DESCRIPTIONS = {
    'read:user': 'Read user account information',
    'write:user': 'Update user account information',
    'read:organizations': "Read user's organizations",
    'read:organization': 'Read organization details',
    'write:organization': 'Update organization',
    'delete:organization': 'Delete organization',
    'read:projects': 'Read organization projects',
    'create:projects': 'Create organization projects',
    'write:projects': 'Update organization projects',
    'delete:projects': 'Delete organization projects',
    'read:members': 'Read organization/project/site members',
    'write:members': 'Manage organization/project/site members',
    'delete:members': 'Remove organization/project/site members',
    'read:invoices': 'Read organization invoices',
    'read:firewall': 'Read firewall rules',
    'write:firewall': 'Update firewall rules',
    'delete:firewall': 'Delete firewall rules',
    'read:site': 'Read site details',
    'write:site': 'Update site configuration',
    'delete:site': 'Delete site',
    'read:sites': 'Read project sites',
    'write:sites': 'Update project sites',
    'delete:sites': 'Delete project sites',
    'promote_sites': 'Promote sites between environments',
    'read:project': 'Read project details',
    'write:project': 'Update project',
    'delete:project': 'Delete project',
    'manage_secrets': 'Read, write, and delete secrets',
    'admin:system': 'Full system administrative access',
}


class ProtoScopeExtractor:
    """Extracts scope annotations from proto files."""

    def __init__(self, proto_dir: Path):
        self.proto_dir = proto_dir
        self.method_scopes: Dict[str, Tuple[str, str, List[str]]] = {}  # method_name -> (resource, level, oauth_scopes)

    def extract_scopes(self) -> Dict[str, Tuple[str, str]]:
        """Extract all scope annotations from proto files."""
        proto_files = list(self.proto_dir.rglob("*.proto"))

        for proto_file in proto_files:
            if 'options' in str(proto_file):
                # Skip option definition files
                continue
            self._parse_proto_file(proto_file)

        return self.method_scopes

    def _parse_proto_file(self, proto_file: Path):
        """Parse a single proto file to extract scope annotations."""
        with open(proto_file, 'r') as f:
            content = f.read()

        # Extract package name
        package_match = re.search(r'package\s+([\w.]+)\s*;', content)
        package_name = package_match.group(1) if package_match else "libops.v1"

        # Find RPC methods with their service context
        # We'll match each rpc individually to avoid nested brace issues
        rpc_pattern = r'service\s+(\w+)\s*\{.*?rpc\s+(\w+)\s*\([^)]+\)\s+returns\s+\([^)]+\)\s*\{([^}]*?)\};'

        # Use a more robust approach: find each service, then find methods within it
        service_starts = [(m.start(), m.group(1)) for m in re.finditer(r'service\s+(\w+)\s*\{', content)]

        for i, (start_pos, service_name) in enumerate(service_starts):
            # Find the end of this service (next service or end of file)
            if i + 1 < len(service_starts):
                end_pos = service_starts[i + 1][0]
            else:
                end_pos = len(content)

            service_content = content[start_pos:end_pos]

            # Find RPC methods in this service
            # Match rpc METHOD_NAME (Req) returns (Res) { options... }
            # The pattern matches from "rpc" to the closing "}" with minimal matching
            rpc_matches = re.finditer(
                r'rpc\s+(\w+)\s*\([^)]+\)\s+returns\s+\([^)]+\)\s*\{(.*?)\n  \}',
                service_content,
                re.DOTALL
            )

            for method_match in rpc_matches:
                method_name = method_match.group(1)
                method_options = method_match.group(2)

                # Extract scope annotation
                scope = self._extract_scope_annotation(method_options)
                if scope:
                    # Build full method name: package.ServiceName/MethodName
                    full_method_name = f"{package_name}.{service_name}/{method_name}"
                    self.method_scopes[full_method_name] = scope

    def _extract_scope_annotation(self, method_options: str) -> Optional[Tuple[str, str, List[str]]]:
        """Extract resource, level, and oauth_scopes from scope annotation."""
        # Look for: option (libops.v1.options.required_scope) = {
        #   resource: RESOURCE_TYPE_XXX
        #   level: ACCESS_LEVEL_XXX
        #   oauth_scopes: "scope1"
        #   oauth_scopes: "scope2"
        # };

        scope_pattern = r'option\s+\(libops\.v1\.options\.required_scope\)\s*=\s*\{([^}]+)\}'
        scope_match = re.search(scope_pattern, method_options)

        if not scope_match:
            return None

        scope_content = scope_match.group(1)

        # Extract resource
        resource_match = re.search(r'resource:\s*(RESOURCE_TYPE_\w+)', scope_content)
        if not resource_match:
            return None
        resource = resource_match.group(1)

        # Extract level
        level_match = re.search(r'level:\s*(ACCESS_LEVEL_\w+)', scope_content)
        if not level_match:
            return None
        level = level_match.group(1)

        # Extract oauth_scopes (can be multiple)
        oauth_scopes = []
        for oauth_match in re.finditer(r'oauth_scopes:\s*"([^"]+)"', scope_content):
            oauth_scopes.append(oauth_match.group(1))

        return (resource, level, oauth_scopes)


class OpenAPIScopesInjector:
    """Injects OAuth scopes into OpenAPI spec."""

    def __init__(self, openapi_file: Path, method_scopes: Dict[str, Tuple[str, str, List[str]]]):
        self.openapi_file = openapi_file
        self.method_scopes = method_scopes
        self.spec = None

    def inject_scopes(self):
        """Inject OAuth scopes into the OpenAPI spec."""
        # Read OpenAPI spec
        with open(self.openapi_file, 'r') as f:
            self.spec = yaml.safe_load(f)

        # Add OAuth2 security scheme
        self._add_security_scheme()

        # Add security requirements to operations
        self._add_operation_security()

        # Write updated spec
        with open(self.openapi_file, 'w') as f:
            yaml.dump(self.spec, f, default_flow_style=False, sort_keys=False)

    def _add_security_scheme(self):
        """Add OAuth2 security scheme to components."""
        if 'components' not in self.spec:
            self.spec['components'] = {}

        if 'securitySchemes' not in self.spec['components']:
            self.spec['components']['securitySchemes'] = {}

        # Build scopes dictionary with all possible scopes
        all_scopes = {}
        for scope_name, description in SCOPE_DESCRIPTIONS.items():
            all_scopes[scope_name] = description

        self.spec['components']['securitySchemes']['oauth2'] = {
            'type': 'oauth2',
            'description': 'OAuth 2.0 authentication via Vault OIDC',
            'flows': {
                'authorizationCode': {
                    'authorizationUrl': '/auth/oauth/authorize',
                    'tokenUrl': '/auth/oauth/token',
                    'scopes': all_scopes
                }
            }
        }

        # Also add API key security scheme
        self.spec['components']['securitySchemes']['apiKey'] = {
            'type': 'http',
            'scheme': 'bearer',
            'bearerFormat': 'API Key',
            'description': 'API key authentication (prefix: libops_)'
        }

    def _add_operation_security(self):
        """Add security requirements to each operation based on scope annotations."""
        if 'paths' not in self.spec:
            return

        for path, path_item in self.spec['paths'].items():
            for method, operation in path_item.items():
                if method in ['get', 'post', 'put', 'patch', 'delete']:
                    self._add_operation_scope(operation, path, method)

    def _add_operation_scope(self, operation: dict, path: str, http_method: str):
        """Add security requirement to a specific operation."""
        # Try to find the corresponding proto method
        # The operationId usually maps to the proto method
        operation_id = operation.get('operationId', '')

        # Find matching proto method
        proto_scope = None
        for method_name, scope in self.method_scopes.items():
            # Match by operation ID (usually ServiceName_MethodName)
            if operation_id and operation_id.replace('_', '') in method_name.replace('/', ''):
                proto_scope = scope
                break

        if proto_scope:
            resource, level, oauth_scopes_from_proto = proto_scope

            # Use OAuth scopes from proto if specified, otherwise fall back to mapping
            if oauth_scopes_from_proto:
                oauth_scopes = oauth_scopes_from_proto
            else:
                oauth_scopes = SCOPE_MAPPING.get((resource, level), [])

            if oauth_scopes:
                # Add security requirement
                # User can authenticate with either OAuth2 OR API key
                operation['security'] = [
                    {'oauth2': oauth_scopes},
                    {'apiKey': []}
                ]

                # Create a readable scope list for display
                scope_list = ', '.join(f'`{s}`' for s in oauth_scopes)
                resource_str = resource.replace('RESOURCE_TYPE_', '').lower()
                level_str = level.replace('ACCESS_LEVEL_', '').lower()

                # Add summary with scope requirements (Mintlify shows summary prominently)
                base_desc = operation.get('description', '').split('\n')[0] if 'description' in operation else ''
                if base_desc:
                    # Append scope list to summary for visibility
                    scope_list_compact = ', '.join(oauth_scopes)
                    operation['summary'] = f"{base_desc} [Requires: {scope_list_compact}]"
                elif 'summary' not in operation:
                    # No existing description, create summary with scopes
                    scope_list_compact = ', '.join(oauth_scopes)
                    operation['summary'] = f"[Requires: {scope_list_compact}]"

                # Add comprehensive scope information to description
                # Simplified format matching user's preference
                scope_info = "\n\n### Authorization\n\n"
                scope_info += "An API key or OAuth token must have at least one of the following scopes in order to use this API endpoint:\n\n"

                # API Key section
                scope_info += "**API Key Scopes**: `"
                scope_info += ", ".join(oauth_scopes)
                scope_info += "`\n"

                # OAuth section with resource hierarchy
                scope_info += "\n**OAuth Scopes**\n\n"
                scope_info += "| Resource | Scope |\n"
                scope_info += "|----------|-------|\n"

                # Map the resource type to show hierarchy
                resource_hierarchy = {
                    'account': ['Account'],
                    'organization': ['Organization'],
                    'project': ['Organization', 'Project'],
                    'site': ['Organization', 'Project', 'Site'],
                }

                resources = resource_hierarchy.get(resource_str, [resource_str.title()])
                for res in resources:
                    for scope in oauth_scopes:
                        scope_info += f"| {res} | `{scope}` |\n"

                if 'description' in operation:
                    operation['description'] += scope_info
                else:
                    operation['description'] = scope_info.strip()

                # Add x-codeSamples extension for better doc tool integration
                operation['x-scopes'] = oauth_scopes
                operation['x-auth-type'] = 'oauth2 or apiKey'
                operation['x-min-access-level'] = f"{resource_str}:{level_str}"


def main():
    """Main entry point."""
    script_dir = Path(__file__).parent
    proto_dir = script_dir / "libops"
    openapi_file = script_dir.parent / "openapi" / "openapi.yaml"

    if not openapi_file.exists():
        print(f"Error: OpenAPI file not found: {openapi_file}", file=sys.stderr)
        sys.exit(1)

    print("Extracting scope annotations from proto files...")
    extractor = ProtoScopeExtractor(proto_dir)
    method_scopes = extractor.extract_scopes()

    print(f"Found {len(method_scopes)} methods with scope annotations")
    for method_name, (resource, level, oauth_scopes) in method_scopes.items():
        scopes_str = f" [{', '.join(oauth_scopes)}]" if oauth_scopes else ""
        print(f"  - {method_name}: {resource}:{level}{scopes_str}")

    print(f"\nInjecting OAuth scopes into {openapi_file}...")
    injector = OpenAPIScopesInjector(openapi_file, method_scopes)
    injector.inject_scopes()

    print("✓ OAuth scopes injected into OpenAPI spec")
    print("✓ Added OAuth2 security scheme")
    print("✓ Added API key security scheme")
    print("✓ Added security requirements to operations")
    print("Done!")


if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)
