#!/usr/bin/env python3
"""
Script to split the merged OpenAPI spec into customer and admin versions.
"""

import yaml
import sys
from pathlib import Path

def split_openapi():
    openapi_dir = Path(__file__).parent.parent / "openapi"
    input_file = openapi_dir / "openapi.yaml"
    customer_file = openapi_dir / "openapi-public.yaml"
    admin_file = openapi_dir / "openapi-admin.yaml"

    print(f"Reading {input_file}...")
    with open(input_file, 'r') as f:
        spec = yaml.safe_load(f)

    # Create customer-only spec
    customer_spec = {
        'openapi': spec['openapi'],
        'info': {
            'title': 'LibOps Customer API',
            'description': 'Customer-facing API for managing your LibOps infrastructure',
            'version': spec['info']['version']
        },
        'paths': {},
        'components': {
            'schemas': {},
            'securitySchemes': spec.get('components', {}).get('securitySchemes', {})
        }
    }

    # Filter paths - only include /v1/* (exclude /admin/v1/*)
    for path, methods in spec.get('paths', {}).items():
        if path.startswith('/v1/') and not path.startswith('/admin/v1/'):
            customer_spec['paths'][path] = methods

    # Filter schemas - only include non-Admin schemas
    for schema_name, schema_def in spec.get('components', {}).get('schemas', {}).items():
        if not schema_name.startswith('Admin'):
            customer_spec['components']['schemas'][schema_name] = schema_def

    # Create admin spec (includes everything)
    admin_spec = spec.copy()
    admin_spec['info'] = {
        'title': 'LibOps Admin API',
        'description': 'Admin API for managing LibOps infrastructure with full access',
        'version': spec['info']['version']
    }

    # Write customer spec
    print(f"Writing {customer_file}...")
    with open(customer_file, 'w') as f:
        yaml.dump(customer_spec, f, default_flow_style=False, sort_keys=False)

    # Write admin spec
    print(f"Writing {admin_file}...")
    with open(admin_file, 'w') as f:
        yaml.dump(admin_spec, f, default_flow_style=False, sort_keys=False)

    print("✓ Created customer-only OpenAPI spec")
    print("✓ Created admin OpenAPI spec with full access")
    print("Done!")

if __name__ == '__main__':
    try:
        split_openapi()
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
