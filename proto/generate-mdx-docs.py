#!/usr/bin/env python3
"""
Generate Mintlify MDX files for all API endpoints from OpenAPI spec.
"""

import yaml
import json
from pathlib import Path
from typing import Dict, List, Tuple


def parse_openapi_spec(openapi_file: Path) -> List[Tuple[str, str, str, str]]:
    """
    Parse OpenAPI spec and return list of (method, path, operation_id, summary).
    Deduplicates endpoints by preferring GET over POST when both exist.
    """
    with open(openapi_file, 'r') as f:
        spec = yaml.safe_load(f)

    endpoints_dict = {}

    for path, path_item in spec.get('paths', {}).items():
        for method in ['get', 'post', 'put', 'patch', 'delete']:
            if method in path_item:
                operation = path_item[method]
                operation_id = operation.get('operationId', '')
                summary = operation.get('summary', '').strip()

                if not summary:
                    description = operation.get('description', '')
                    summary = description.split('\n')[0] if description else ''

                clean_operation_id = operation_id.replace('.get', '')

                if clean_operation_id not in endpoints_dict or method == 'get':
                    endpoints_dict[clean_operation_id] = (method.upper(), path, clean_operation_id, summary)

    return list(endpoints_dict.values())


def operation_id_to_filename(operation_id: str) -> str:
    """
    Convert ConnectRPC operation ID to filename.
    Format: libops.v1.AccountService.CreateApiKey -> create_api_key
    """
    import re

    # Extract method name from operation ID
    parts = operation_id.split('.')
    method_name = parts[-1]

    # Convert camelCase to snake_case
    filename = re.sub('([A-Z]+)', r'_\1', method_name).lower()
    return filename.lstrip('_')


def categorize_endpoint(path: str, operation_id: str) -> Tuple[str, str]:
    """
    Categorize ConnectRPC endpoint into group and subgroup.
    Path format: /libops.v1.ServiceName/MethodName
    Returns: (group, subgroup) where subgroup can be None
    """
    service_path = path.split('/')[1]
    service_name = service_path.split('.')[-1]

    if service_name == 'OrganizationService':
        return ('Organizations', None)
    elif service_name == 'ProjectService':
        return ('Projects', None)
    elif service_name == 'SiteService':
        return ('Sites', None)
    elif service_name == 'FirewallService':
        return ('Firewall', 'Organization Firewall')
    elif service_name == 'ProjectFirewallService':
        return ('Firewall', 'Project Firewall')
    elif service_name == 'SiteFirewallService':
        return ('Firewall', 'Site Firewall')
    elif service_name == 'MemberService':
        return ('Members', 'Organization Members')
    elif service_name == 'ProjectMemberService':
        return ('Members', 'Project Members')
    elif service_name == 'SiteMemberService':
        return ('Members', 'Site Members')
    elif service_name == 'OrganizationSecretService':
        return ('Secrets', 'Organization Secrets')
    elif service_name == 'ProjectSecretService':
        return ('Secrets', 'Project Secrets')
    elif service_name == 'SiteSecretService':
        return ('Secrets', 'Site Secrets')
    elif service_name == 'SshKeyService':
        return ('SSH Keys', None)
    elif service_name == 'AccountService':
        return ('Account', None)
    elif service_name == 'SiteOperationsService':
        return ('Site Operations', None)
    elif service_name.startswith('Admin'):
        base_service = service_name.replace('Admin', '')
        return (f'Admin {base_service.replace("Service", "")}', None)
    else:
        return (service_name.replace('Service', ''), None)


def generate_mdx_file(method: str, path: str, summary: str, output_dir: Path, filename: str):
    """Generate a single MDX file."""
    title_quoted = f'"{summary}"' if "'" in summary else f"'{summary}'"

    content = f"""---
title: {title_quoted}
openapi: '{method} {path}'
---
"""

    output_file = output_dir / f"{filename}.mdx"
    with open(output_file, 'w') as f:
        f.write(content)

    return output_file


def build_navigation_structure(endpoints: List[Tuple[str, str, str, str]]) -> Dict:
    """Build navigation structure for docs.json."""
    groups = {}

    for method, path, operation_id, summary in endpoints:
        group, subgroup = categorize_endpoint(path, operation_id)
        filename = operation_id_to_filename(operation_id)
        page_path = f"api/reference/{filename}"

        if group not in groups:
            groups[group] = {}

        if subgroup:
            if subgroup not in groups[group]:
                groups[group][subgroup] = []
            groups[group][subgroup].append(page_path)
        else:
            if '__main__' not in groups[group]:
                groups[group]['__main__'] = []
            groups[group]['__main__'].append(page_path)

    nav_groups = []
    nav_groups.append({
        "group": "API documentation",
        "pages": ["api/reference/introduction"]
    })

    group_order = [
        'Organizations',
        'Projects',
        'Sites',
        'Firewall',
        'Members',
        'Secrets',
        'SSH Keys',
        'Site Operations',
        'Account'
    ]

    for group_name in group_order:
        if group_name not in groups:
            continue

        group_data = groups[group_name]
        has_subgroups = any(k != '__main__' for k in group_data.keys())

        if has_subgroups:
            subgroup_pages = []
            for subgroup_name, pages in sorted(group_data.items()):
                if subgroup_name == '__main__':
                    continue
                subgroup_pages.append({
                    "group": subgroup_name,
                    "pages": sorted(pages)
                })

            if '__main__' in group_data:
                for page in sorted(group_data['__main__']):
                    subgroup_pages.insert(0, page)

            nav_groups.append({
                "group": group_name,
                "pages": subgroup_pages
            })
        else:
            pages = group_data.get('__main__', [])
            nav_groups.append({
                "group": group_name,
                "pages": sorted(pages)
            })

    return nav_groups


def main():
    script_dir = Path(__file__).parent
    openapi_file = script_dir.parent / "openapi" / "openapi.yaml"
    docs_dir = Path("../docs")
    output_dir = docs_dir / "api" / "reference"
    docs_json = docs_dir / "docs.json"

    output_dir.mkdir(parents=True, exist_ok=True)

    print("Parsing OpenAPI spec...")
    endpoints = parse_openapi_spec(openapi_file)
    print(f"Found {len(endpoints)} endpoints")

    print("\nGenerating MDX files...")
    generated_files = []
    for method, path, operation_id, summary in endpoints:
        filename = operation_id_to_filename(operation_id)
        output_file = generate_mdx_file(method, path, summary, output_dir, filename)
        generated_files.append(output_file)
        print(f"  ✓ {filename}.mdx")

    print(f"\n✓ Generated {len(generated_files)} MDX files")

    print("\nBuilding navigation structure...")
    nav_groups = build_navigation_structure(endpoints)

    print("Updating docs.json...")
    with open(docs_json, 'r') as f:
        docs_config = json.load(f)

    for tab in docs_config['navigation']['tabs']:
        if tab.get('tab') == 'API reference':
            tab['groups'] = nav_groups
            break

    with open(docs_json, 'w') as f:
        json.dump(docs_config, f, indent=2)

    print("✓ Updated docs.json")
    print(f"\n✅ Complete! Generated {len(generated_files)} endpoint docs")


if __name__ == '__main__':
    main()
