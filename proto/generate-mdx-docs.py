#!/usr/bin/env python3
"""
Generate Mintlify MDX files for all API endpoints from OpenAPI spec.
"""

import yaml
import json
from pathlib import Path
from typing import Dict, List, Tuple

# Map HTTP methods to readable titles
METHOD_NAMES = {
    'get': 'Get',
    'post': 'Create',
    'put': 'Update',
    'patch': 'Update',
    'delete': 'Delete'
}

def parse_openapi_spec(openapi_file: Path) -> List[Tuple[str, str, str, str]]:
    """
    Parse OpenAPI spec and return list of (method, path, operation_id, summary).
    """
    with open(openapi_file, 'r') as f:
        spec = yaml.safe_load(f)

    endpoints = []

    for path, path_item in spec.get('paths', {}).items():
        for method in ['get', 'post', 'put', 'patch', 'delete']:
            if method in path_item:
                operation = path_item[method]
                operation_id = operation.get('operationId', '')
                summary = operation.get('summary', '').split('[')[0].strip()  # Remove [Requires: ...] part

                if not summary:
                    # Extract from description
                    description = operation.get('description', '')
                    summary = description.split('\n')[0] if description else ''

                endpoints.append((method.upper(), path, operation_id, summary))

    return endpoints


def operation_id_to_filename(operation_id: str) -> str:
    """
    Convert operation ID to filename.
    Example: OrganizationService_GetOrganization -> get_organization
    """
    # Remove service prefix
    if '_' in operation_id:
        parts = operation_id.split('_', 1)
        method_name = parts[1] if len(parts) > 1 else parts[0]
    else:
        method_name = operation_id

    # Convert camelCase to snake_case
    import re
    filename = re.sub('([A-Z]+)', r'_\1', method_name).lower()
    filename = filename.lstrip('_')

    return filename


def categorize_endpoint(path: str, operation_id: str) -> Tuple[str, str]:
    """
    Categorize endpoint into group and subgroup.
    Returns: (group, subgroup)
    """
    # Determine main resource
    if '/organizations/' in path:
        if '/firewall' in path:
            return ('Firewall', 'Organization Firewall')
        elif '/members' in path:
            return ('Members', 'Organization Members')
        elif '/secrets' in path:
            return ('Secrets', 'Organization Secrets')
        elif '/projects' in path and path.endswith('/projects'):
            return ('Organizations', None)
        else:
            return ('Organizations', None)
    elif '/projects/' in path:
        if '/firewall' in path:
            return ('Firewall', 'Project Firewall')
        elif '/members' in path:
            return ('Members', 'Project Members')
        elif '/secrets' in path:
            return ('Secrets', 'Project Secrets')
        elif '/sites' in path and path.endswith('/sites'):
            return ('Projects', None)
        else:
            return ('Projects', None)
    elif '/sites/' in path:
        if '/firewall' in path:
            return ('Firewall', 'Site Firewall')
        elif '/members' in path:
            return ('Members', 'Site Members')
        elif '/secrets' in path:
            return ('Secrets', 'Site Secrets')
        elif '/deploy' in path or '/status' in path:
            return ('Site Operations', None)
        else:
            return ('Sites', None)
    elif path == '/v1/organizations':
        return ('Organizations', None)
    elif path == '/v1/projects':
        return ('Projects', None)
    elif path == '/v1/sites':
        return ('Sites', None)
    elif '/ssh-keys' in path:
        return ('SSH Keys', None)
    elif '/accounts/' in path:
        return ('Accounts', None)
    else:
        return ('General', None)


def generate_mdx_file(method: str, path: str, summary: str, output_dir: Path, filename: str):
    """Generate a single MDX file."""
    # Use summary as title, or generate from path
    if summary:
        title = summary
    else:
        # Generate title from path
        parts = path.strip('/').split('/')
        resource = parts[-1] if parts else 'endpoint'
        title = f"{METHOD_NAMES.get(method.lower(), method)} {resource.replace('_', ' ').title()}"

    # Use double quotes if title contains apostrophe, otherwise single quotes
    if "'" in title:
        title_quoted = f'"{title}"'
    else:
        title_quoted = f"'{title}'"

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
    """
    Build navigation structure for docs.json.
    Returns grouped navigation structure.
    """
    # Group endpoints by category
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

    # Build navigation JSON structure
    nav_groups = []

    # Add introduction first
    nav_groups.append({
        "group": "API documentation",
        "pages": ["api/reference/introduction"]
    })

    # Order of groups
    group_order = [
        'Organizations',
        'Projects',
        'Sites',
        'Firewall',
        'Members',
        'Secrets',
        'SSH Keys',
        'Site Operations',
        'Accounts'
    ]

    for group_name in group_order:
        if group_name not in groups:
            continue

        group_data = groups[group_name]

        # Check if this group has subgroups
        has_subgroups = any(k != '__main__' for k in group_data.keys())

        if has_subgroups:
            # Group with subgroups
            subgroup_pages = []
            for subgroup_name, pages in sorted(group_data.items()):
                if subgroup_name == '__main__':
                    continue
                subgroup_pages.append({
                    "group": subgroup_name,
                    "pages": sorted(pages)
                })

            # Add main pages if any
            if '__main__' in group_data:
                for page in sorted(group_data['__main__']):
                    subgroup_pages.insert(0, page)

            nav_groups.append({
                "group": group_name,
                "pages": subgroup_pages
            })
        else:
            # Simple group without subgroups
            pages = group_data.get('__main__', [])
            nav_groups.append({
                "group": group_name,
                "pages": sorted(pages)
            })

    return nav_groups


def main():
    # Paths
    script_dir = Path(__file__).parent
    openapi_file = script_dir.parent / "openapi" / "openapi-customer.yaml"
    docs_dir = Path("/Users/jcorall/libops/docs")
    output_dir = docs_dir / "api" / "reference"
    docs_json = docs_dir / "docs.json"

    # Create output directory
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

    # Find API reference tab and update its groups
    for tab in docs_config['navigation']['tabs']:
        if tab.get('tab') == 'API reference':
            tab['groups'] = nav_groups
            break

    # Write updated docs.json
    with open(docs_json, 'w') as f:
        json.dump(docs_config, f, indent=2)

    print("✓ Updated docs.json")
    print(f"\n✅ Complete! Generated {len(generated_files)} endpoint docs")


if __name__ == '__main__':
    main()
