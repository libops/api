#!/usr/bin/env python3
"""
Bulk Test Data Generator for LibOps Platform
Generates hundreds of organizations with Seinfeld and Twin Peaks themed data
"""

import random
import hashlib
from datetime import datetime, timedelta

# Seinfeld Characters (Main + Supporting + Minor)
SEINFELD_CHARACTERS = [
    ("Frank", "Costanza", "frank.costanza"),
    ("Estelle", "Costanza", "estelle.costanza"),
    ("Morty", "Seinfeld", "morty.seinfeld"),
    ("Helen", "Seinfeld", "helen.seinfeld"),
    ("Susan", "Ross", "susan.ross"),
    ("Uncle Leo", "", "uncle.leo"),
    ("Kenny", "Bania", "kenny.bania"),
    ("Puddy", "David", "david.puddy"),
    ("Mr.", "Pitt", "mr.pitt"),
    ("Steinbrenner", "George", "george.steinbrenner"),
    ("Mr.", "Wilhelm", "mr.wilhelm"),
    ("Lloyd", "Braun", "lloyd.braun"),
    ("David", "Puddy", "david.puddy"),
    ("Tim", "Whatley", "tim.whatley"),
    ("Izzy", "Mandelbaum", "izzy.mandelbaum"),
    ("Donna", "Chang", "donna.chang"),
    ("Alton", "Benes", "alton.benes"),
    ("Justin", "Pitt", "justin.pitt"),
    ("The", "Drake", "the.drake"),
    ("Mickey", "Abbott", "mickey.abbott"),
    ("Bette", "Midler", "bette.midler"),
    ("Keith", "Hernandez", "keith.hernandez"),
    ("Russell", "Dalrymple", "russell.dalrymple"),
    ("Jack", "Klompus", "jack.klompus"),
    ("Morty", "Seinfeld", "morty.seinfeld"),
    ("Mr.", "Lippman", "mr.lippman"),
    ("Jake", "Jarmel", "jake.jarmel"),
    ("Joel", "Rifkin", "joel.rifkin"),
    ("The", "Maestro", "the.maestro"),
    ("The", "Wiz", "the.wiz"),
    ("Sue Ellen", "Mischke", "sue.ellen.mischke"),
    ("Ramon", "", "ramon"),
    ("Marcelino", "", "marcelino"),
    ("Lomez", "", "lomez"),
    ("Sally", "Weaver", "sally.weaver"),
    ("Rachel", "", "rachel"),
]

# Twin Peaks Characters
TWIN_PEAKS_CHARACTERS = [
    ("Dale", "Cooper", "dale.cooper"),
    ("Laura", "Palmer", "laura.palmer"),
    ("Harry", "Truman", "harry.truman"),
    ("Audrey", "Horne", "audrey.horne"),
    ("Bobby", "Briggs", "bobby.briggs"),
    ("Shelly", "Johnson", "shelly.johnson"),
    ("James", "Hurley", "james.hurley"),
    ("Donna", "Hayward", "donna.hayward"),
    ("Big Ed", "Hurley", "biged.hurley"),
    ("Norma", "Jennings", "norma.jennings"),
    ("Log Lady", "Margaret", "margaret.loglady"),
    ("Ben", "Horne", "ben.horne"),
    ("Leland", "Palmer", "leland.palmer"),
    ("Maddy", "Ferguson", "maddy.ferguson"),
    ("Leo", "Johnson", "leo.johnson"),
    ("Josie", "Packard", "josie.packard"),
    ("Catherine", "Martell", "catherine.martell"),
    ("Pete", "Martell", "pete.martell"),
    ("Andy", "Brennan", "andy.brennan"),
    ("Lucy", "Moran", "lucy.moran"),
    ("Albert", "Rosenfield", "albert.rosenfield"),
    ("Gordon", "Cole", "gordon.cole"),
    ("Phillip", "Jeffries", "phillip.jeffries"),
    ("Major", "Briggs", "major.briggs"),
    ("Sarah", "Palmer", "sarah.palmer"),
    ("Jacques", "Renault", "jacques.renault"),
    ("Jean", "Renault", "jean.renault"),
    ("Bernard", "Renault", "bernard.renault"),
    ("Hank", "Jennings", "hank.jennings"),
    ("Eileen", "Hayward", "eileen.hayward"),
    ("Doc", "Hayward", "doc.hayward"),
    ("Dick", "Tremayne", "dick.tremayne"),
    ("Windom", "Earle", "windom.earle"),
    ("Annie", "Blackburn", "annie.blackburn"),
]

# Seinfeld-themed Organization Names
SEINFELD_ORGS = [
    "Pendant Publishing",
    "Kruger Industrial Smoothing",
    "Play Now",
    "The Human Fund",
    "Peterman Reality Tour",
    "J. Peterman Catalog",
    "Monk's Cafe Holdings",
    "Del Boca Vista Properties",
    "Festivus Solutions",
    "Costanza & Son Architecture",
    "La Cocina Restaurant Group",
    "The Big Salad Co",
    "Fusilli Jerry Productions",
    "Marble Rye Bakery",
    "Junior Mints Manufacturing",
    "Kenny Rogers Roasters Franchise",
    "The Nexus of the Universe LLC",
    "Assman Logistics",
    "Serenity Now Wellness",
    "Hoochie Mama Enterprises",
    "The Jerk Store",
    "Top of the Muffin",
    "Penske File Management",
    "Yankee Stadium Operations",
    "The Drake's Wedding Co",
    "Bra nia Comedy Circuit",
    "The Library Investigation Services",
    "Parking Garage Solutions",
    "Chinese Restaurant Holdings",
    "The Soup Stand",
]

# Twin Peaks themed Organization Names
TWIN_PEAKS_ORGS = [
    "Great Northern Hotel",
    "Double R Diner",
    "Packard Sawmill",
    "Horne's Department Store",
    "One Eyed Jacks Casino",
    "Blue Pine Lodge",
    "Ghostwood Development",
    "Bookhouse Boys Security",
    "Twin Peaks Sheriff Department",
    "Palmer Family Trust",
    "Briggs Air Force Research",
    "Black Lodge Industries",
    "White Lodge Foundation",
    "Owl Cave Mining",
    "Sparkwood & 21 Holdings",
    "The Log Lady's Tours",
    "Jacques Renault Enterprises",
    "Dead Dog Farm",
    "Wind River Properties",
]

# Project names
PROJECT_NAMES = [
    "Neptune", "Venus", "Mars", "Mercury", "Saturn", "Uranus", "Pluto",
    "Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Eta",
    "Phoenix", "Dragon", "Griffin", "Pegasus", "Chimera",
    "Quantum", "Nebula", "Galaxy", "Cosmos", "Stellar",
    "Apex", "Zenith", "Vertex", "Summit", "Peak"
]

# Site environment names
SITE_ENVS = ["production", "staging", "development", "qa", "demo", "sandbox"]

def generate_uuid_from_seed(prefix, seed):
    """Generate a deterministic UUID from a seed"""
    hash_obj = hashlib.md5(f"{prefix}-{seed}".encode())
    hex_str = hash_obj.hexdigest()
    return f"{prefix}-{hex_str[8:12]}-{hex_str[12:16]}-{hex_str[16:20]}-{hex_str[20:32]}"

def sanitize_name(name):
    """Sanitize name for use in emails and IDs"""
    return name.lower().replace(" ", ".").replace("'", "").replace(".", "")

def main():
    # Configuration
    NUM_ORGS = 200
    MIN_PROJECTS_PER_ORG = 0
    MAX_PROJECTS_PER_ORG = 20
    MIN_SITES_PER_PROJECT = 0
    MAX_SITES_PER_PROJECT = 5
    MIN_MEMBERS_PER_RESOURCE = 0
    MAX_MEMBERS_PER_RESOURCE = 10

    # Starting IDs
    account_id = 100
    org_id = 100
    project_id = 100
    site_id = 100
    relationship_id = 100

    # Combine character pools
    all_characters = SEINFELD_CHARACTERS + TWIN_PEAKS_CHARACTERS
    all_org_names = SEINFELD_ORGS + TWIN_PEAKS_ORGS

    # Track used characters
    character_index = 0

    output = []
    output.append("-- ============================================")
    output.append("-- BULK TEST DATA - AUTO GENERATED")
    output.append(f"-- Generated: {datetime.now().isoformat()}")
    output.append(f"-- Organizations: {NUM_ORGS}")
    output.append("-- ============================================\n")

    # Generate Organizations
    for org_num in range(NUM_ORGS):
        org_name = all_org_names[org_num % len(all_org_names)]
        if org_num >= len(all_org_names):
            org_name = f"{org_name} #{org_num // len(all_org_names) + 1}"

        org_uuid = generate_uuid_from_seed("0rg00000", f"bulk-{org_id}")

        # Pick an owner for this org
        if character_index >= len(all_characters):
            character_index = 0

        owner_first, owner_last, owner_username = all_characters[character_index]
        character_index += 1

        owner_id = account_id
        account_id += 1

        owner_name = f"{owner_first} {owner_last}".strip()
        owner_email = f"{owner_username}@{sanitize_name(org_name)}.com"
        owner_uuid = generate_uuid_from_seed("acc00000", f"bulk-{owner_id}")

        # Create owner account
        output.append(f"-- Organization: {org_name}")
        output.append(f"INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)")
        output.append(f"VALUES (")
        output.append(f"    {owner_id},")
        output.append(f"    UNHEX(REPLACE('{owner_uuid}', '-', '')),")
        output.append(f"    '{owner_email}',")
        output.append(f"    '{owner_name}',")
        output.append(f"    'apikey',")
        output.append(f"    TRUE,")
        output.append(f"    'entity-bulk-{owner_id}',")
        output.append(f"    NOW()")
        output.append(f");")
        output.append("")

        # Create API key for owner
        apikey_uuid = generate_uuid_from_seed("ap1key00", f"bulk-{owner_id}")
        output.append(f"INSERT INTO api_keys (public_id, account_id, name, description, scopes, active, created_by, created_at)")
        output.append(f"VALUES (")
        output.append(f"    UNHEX(REPLACE('{apikey_uuid}', '-', '')),")
        output.append(f"    {owner_id},")
        output.append(f"    '{owner_name} Full Access',")
        output.append(f"    'Auto-generated bulk data',")
        output.append(f"    '[]',")
        output.append(f"    TRUE,")
        output.append(f"    {owner_id},")
        output.append(f"    NOW()")
        output.append(f");")
        output.append("")

        # Create organization
        gcp_org_id = 1000000000 + org_id
        gcp_folder_id = 2000000000 + org_id
        gcp_project_num = 3000000000 + org_id

        output.append(f"INSERT INTO organizations (id, public_id, name, gcp_org_id, gcp_billing_account, gcp_parent, location, region, gcp_folder_id, status, gcp_project_id, gcp_project_number, created_by, created_at)")
        output.append(f"VALUES (")
        output.append(f"    {org_id},")
        output.append(f"    UNHEX(REPLACE('{org_uuid}', '-', '')),")
        output.append(f"    '{org_name}',")
        output.append(f"    '{gcp_org_id}',")
        output.append(f"    'BULK-BILLING-{org_id}',")
        output.append(f"    'organizations/{gcp_org_id}',")
        output.append(f"    'us',")
        output.append(f"    'us-{['central', 'east', 'west'][org_id % 3]}1',")
        output.append(f"    'folders/{gcp_folder_id}',")
        output.append(f"    'active',")
        output.append(f"    '{sanitize_name(org_name)}-platform',")
        output.append(f"    '{gcp_project_num}',")
        output.append(f"    {owner_id},")
        output.append(f"    NOW()")
        output.append(f");")
        output.append("")

        # Create relationship to LibOps root org (org_id=1)
        rel_uuid = generate_uuid_from_seed("re1at000", f"bulk-{relationship_id}")
        output.append(f"INSERT INTO relationships (id, public_id, source_organization_id, target_organization_id, relationship_type, status)")
        output.append(f"VALUES (")
        output.append(f"    {relationship_id},")
        output.append(f"    UNHEX(REPLACE('{rel_uuid}', '-', '')),")
        output.append(f"    1,")  # LibOps Platform
        output.append(f"    {org_id},")
        output.append(f"    'access',")
        output.append(f"    'approved'")
        output.append(f");")
        output.append("")
        relationship_id += 1

        # Add owner as organization member
        orgmem_uuid = generate_uuid_from_seed("0rgmem00", f"bulk-{org_id}-{owner_id}")
        output.append(f"INSERT INTO organization_members (public_id, organization_id, account_id, role, status, created_by, created_at)")
        output.append(f"VALUES (")
        output.append(f"    UUID_TO_BIN('{orgmem_uuid}'),")
        output.append(f"    {org_id},")
        output.append(f"    {owner_id},")
        output.append(f"    'owner',")
        output.append(f"    'active',")
        output.append(f"    {owner_id},")
        output.append(f"    NOW()")
        output.append(f");")
        output.append("")

        # Additional org members
        num_additional_members = random.randint(MIN_MEMBERS_PER_RESOURCE, min(5, MAX_MEMBERS_PER_RESOURCE))
        for mem_num in range(num_additional_members):
            if character_index >= len(all_characters):
                character_index = 0

            mem_first, mem_last, mem_username = all_characters[character_index]
            character_index += 1

            mem_id = account_id
            account_id += 1

            mem_name = f"{mem_first} {mem_last}".strip()
            mem_email = f"{mem_username}@{sanitize_name(org_name)}.com"
            mem_uuid = generate_uuid_from_seed("acc00000", f"bulk-{mem_id}")
            mem_role = random.choice(['developer', 'developer', 'read'])  # Weight towards developer

            # Create member account
            output.append(f"INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)")
            output.append(f"VALUES ({mem_id}, UNHEX(REPLACE('{mem_uuid}', '-', '')), '{mem_email}', '{mem_name}', 'apikey', TRUE, 'entity-bulk-{mem_id}', NOW());")
            output.append("")

            # Add as org member
            orgmem_uuid = generate_uuid_from_seed("0rgmem00", f"bulk-{org_id}-{mem_id}")
            output.append(f"INSERT INTO organization_members (public_id, organization_id, account_id, role, status, created_by, created_at)")
            output.append(f"VALUES (UUID_TO_BIN('{orgmem_uuid}'), {org_id}, {mem_id}, '{mem_role}', 'active', {owner_id}, NOW());")
            output.append("")

        # Generate Projects for this org
        num_projects = random.randint(MIN_PROJECTS_PER_ORG, MAX_PROJECTS_PER_ORG)
        for proj_num in range(num_projects):
            project_name = f"Project {PROJECT_NAMES[proj_num % len(PROJECT_NAMES)]}"
            if proj_num >= len(PROJECT_NAMES):
                project_name = f"{project_name} #{proj_num // len(PROJECT_NAMES) + 1}"

            proj_uuid = generate_uuid_from_seed("pr0j0000", f"bulk-{project_id}")
            proj_gcp_num = 4000000000 + project_id

            output.append(f"-- Project: {project_name} ({org_name})")
            output.append(f"INSERT INTO projects (id, public_id, organization_id, name, github_repository, github_branch, gcp_region, gcp_zone, machine_type, gcp_project_id, gcp_project_number, status, created_by, created_at)")
            output.append(f"VALUES (")
            output.append(f"    {project_id},")
            output.append(f"    UNHEX(REPLACE('{proj_uuid}', '-', '')),")
            output.append(f"    {org_id},")
            output.append(f"    '{project_name}',")
            output.append(f"    '{sanitize_name(org_name)}/{sanitize_name(project_name)}',")
            output.append(f"    'main',")
            output.append(f"    'us-{['central', 'east', 'west'][project_id % 3]}1',")
            output.append(f"    'us-{['central', 'east', 'west'][project_id % 3]}1-{chr(97 + (project_id % 3))}',")
            output.append(f"    'e2-{['micro', 'small', 'medium', 'standard-2'][project_id % 4]}',")
            output.append(f"    '{sanitize_name(org_name)}-{sanitize_name(project_name)}',")
            output.append(f"    '{proj_gcp_num}',")
            output.append(f"    'active',")
            output.append(f"    {owner_id},")
            output.append(f"    NOW()")
            output.append(f");")
            output.append("")

            # Project members
            num_proj_members = random.randint(MIN_MEMBERS_PER_RESOURCE, min(8, MAX_MEMBERS_PER_RESOURCE))
            for proj_mem_num in range(num_proj_members):
                if character_index >= len(all_characters):
                    character_index = 0

                proj_mem_first, proj_mem_last, proj_mem_username = all_characters[character_index]
                character_index += 1

                proj_mem_id = account_id
                account_id += 1

                proj_mem_name = f"{proj_mem_first} {proj_mem_last}".strip()
                proj_mem_email = f"{proj_mem_username}@{sanitize_name(org_name)}.com"
                proj_mem_uuid = generate_uuid_from_seed("acc00000", f"bulk-{proj_mem_id}")
                proj_mem_role = 'owner' if proj_mem_num == 0 else random.choice(['developer', 'developer', 'read'])

                # Create member account
                output.append(f"INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)")
                output.append(f"VALUES ({proj_mem_id}, UNHEX(REPLACE('{proj_mem_uuid}', '-', '')), '{proj_mem_email}', '{proj_mem_name}', 'apikey', TRUE, 'entity-bulk-{proj_mem_id}', NOW());")
                output.append("")

                # Add as project member
                projmem_uuid = generate_uuid_from_seed("pr0jmem0", f"bulk-{project_id}-{proj_mem_id}")
                output.append(f"INSERT INTO project_members (public_id, project_id, account_id, role, status, created_by, created_at)")
                output.append(f"VALUES (UUID_TO_BIN('{projmem_uuid}'), {project_id}, {proj_mem_id}, '{proj_mem_role}', 'active', {owner_id}, NOW());")
                output.append("")

            # Generate Sites for this project
            num_sites = random.randint(MIN_SITES_PER_PROJECT, MAX_SITES_PER_PROJECT)
            for site_num in range(num_sites):
                site_name = SITE_ENVS[site_num % len(SITE_ENVS)]
                if site_num >= len(SITE_ENVS):
                    site_name = f"{site_name}-{site_num // len(SITE_ENVS) + 1}"

                site_uuid = generate_uuid_from_seed("51te0000", f"bulk-{site_id}")
                site_ip = f"35.{random.randint(190, 250)}.{random.randint(1, 254)}.{random.randint(1, 254)}"

                output.append(f"-- Site: {site_name} ({project_name})")
                output.append(f"INSERT INTO sites (id, public_id, project_id, name, github_ref, gcp_external_ip, status, created_by, created_at)")
                output.append(f"VALUES (")
                output.append(f"    {site_id},")
                output.append(f"    UNHEX(REPLACE('{site_uuid}', '-', '')),")
                output.append(f"    {project_id},")
                output.append(f"    '{site_name}',")
                output.append(f"    'tags/v{random.randint(1, 5)}.{random.randint(0, 20)}.{random.randint(0, 10)}',")
                output.append(f"    '{site_ip}',")
                output.append(f"    'active',")
                output.append(f"    {owner_id},")
                output.append(f"    NOW()")
                output.append(f");")
                output.append("")

                # Site members
                num_site_members = random.randint(MIN_MEMBERS_PER_RESOURCE, min(6, MAX_MEMBERS_PER_RESOURCE))
                for site_mem_num in range(num_site_members):
                    if character_index >= len(all_characters):
                        character_index = 0

                    site_mem_first, site_mem_last, site_mem_username = all_characters[character_index]
                    character_index += 1

                    site_mem_id = account_id
                    account_id += 1

                    site_mem_name = f"{site_mem_first} {site_mem_last}".strip()
                    site_mem_email = f"{site_mem_username}@{sanitize_name(org_name)}.com"
                    site_mem_uuid = generate_uuid_from_seed("acc00000", f"bulk-{site_mem_id}")
                    site_mem_role = 'owner' if site_mem_num == 0 else random.choice(['developer', 'developer', 'read'])

                    # Create member account
                    output.append(f"INSERT INTO accounts (id, public_id, email, name, auth_method, verified, vault_entity_id, created_at)")
                    output.append(f"VALUES ({site_mem_id}, UNHEX(REPLACE('{site_mem_uuid}', '-', '')), '{site_mem_email}', '{site_mem_name}', 'apikey', TRUE, 'entity-bulk-{site_mem_id}', NOW());")
                    output.append("")

                    # Add as site member
                    sitemem_uuid = generate_uuid_from_seed("51temem0", f"bulk-{site_id}-{site_mem_id}")
                    output.append(f"INSERT INTO site_members (public_id, site_id, account_id, role, status, created_by, created_at)")
                    output.append(f"VALUES (UUID_TO_BIN('{sitemem_uuid}'), {site_id}, {site_mem_id}, '{site_mem_role}', 'active', {owner_id}, NOW());")
                    output.append("")

                site_id += 1

            project_id += 1

        org_id += 1
        output.append("")

    # Write to file
    with open('rbac_bulk_seed.sql', 'w') as f:
        f.write('\n'.join(output))

    print(f"Generated bulk seed data:")
    print(f"  - {NUM_ORGS} organizations")
    print(f"  - {account_id - 100} total accounts")
    print(f"  - {project_id - 100} total projects")
    print(f"  - {site_id - 100} total sites")
    print(f"  - {relationship_id - 100} relationships")
    print(f"Output written to: rbac_bulk_seed.sql")

if __name__ == "__main__":
    main()
