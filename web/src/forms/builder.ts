// Proto-based form builder

import { getPageContext } from "@/utils/context";
import { showLoadingModal, closeModal } from "@/utils/modal";
import { showNotification, capitalize, singularize } from "@/utils/helpers";
import { organizationClient, projectClient } from "@/api/client";
import {
  createOrganization,
  createProject,
  createSite,
  createFirewallRule,
  createMember,
  createSecret,
  createSetting,
  updateOrganization,
  updateProject,
  updateSite,
  updateMember,
  updateSecret,
  updateSetting,
} from "@/resources/operations";

interface FormField {
  name: string;
  label: string;
  type: string;
  required: boolean;
  placeholder?: string;
  options?: { value: string; label: string }[];
}

// GCP region to zones mapping
const regionZones: Record<string, string[]> = {
  "us-central1": ["us-central1-a", "us-central1-b", "us-central1-c", "us-central1-f"],
  "us-east1": ["us-east1-b", "us-east1-c", "us-east1-d"],
  "us-west1": ["us-west1-a", "us-west1-b", "us-west1-c"],
  "us-west2": ["us-west2-a", "us-west2-b", "us-west2-c"],
  "us-west3": ["us-west3-a", "us-west3-b", "us-west3-c"],
  "us-west4": ["us-west4-a", "us-west4-b", "us-west4-c"],
  "us-east4": ["us-east4-a", "us-east4-b", "us-east4-c"],
  "us-south1": ["us-south1-a", "us-south1-b", "us-south1-c"],
  "northamerica-northeast1": ["northamerica-northeast1-a", "northamerica-northeast1-b", "northamerica-northeast1-c"],
  "northamerica-northeast2": ["northamerica-northeast2-a", "northamerica-northeast2-b", "northamerica-northeast2-c"],
  "southamerica-east1": ["southamerica-east1-a", "southamerica-east1-b", "southamerica-east1-c"],
  "southamerica-west1": ["southamerica-west1-a", "southamerica-west1-b", "southamerica-west1-c"],
  "europe-west1": ["europe-west1-b", "europe-west1-c", "europe-west1-d"],
  "europe-west2": ["europe-west2-a", "europe-west2-b", "europe-west2-c"],
  "europe-west3": ["europe-west3-a", "europe-west3-b", "europe-west3-c"],
  "europe-west4": ["europe-west4-a", "europe-west4-b", "europe-west4-c"],
  "europe-west6": ["europe-west6-a", "europe-west6-b", "europe-west6-c"],
  "europe-west8": ["europe-west8-a", "europe-west8-b", "europe-west8-c"],
  "europe-west9": ["europe-west9-a", "europe-west9-b", "europe-west9-c"],
  "europe-central2": ["europe-central2-a", "europe-central2-b", "europe-central2-c"],
  "europe-north1": ["europe-north1-a", "europe-north1-b", "europe-north1-c"],
  "europe-southwest1": ["europe-southwest1-a", "europe-southwest1-b", "europe-southwest1-c"],
  "asia-east1": ["asia-east1-a", "asia-east1-b", "asia-east1-c"],
  "asia-east2": ["asia-east2-a", "asia-east2-b", "asia-east2-c"],
  "asia-northeast1": ["asia-northeast1-a", "asia-northeast1-b", "asia-northeast1-c"],
  "asia-northeast2": ["asia-northeast2-a", "asia-northeast2-b", "asia-northeast2-c"],
  "asia-northeast3": ["asia-northeast3-a", "asia-northeast3-b", "asia-northeast3-c"],
  "asia-south1": ["asia-south1-a", "asia-south1-b", "asia-south1-c"],
  "asia-south2": ["asia-south2-a", "asia-south2-b", "asia-south2-c"],
  "asia-southeast1": ["asia-southeast1-a", "asia-southeast1-b", "asia-southeast1-c"],
  "asia-southeast2": ["asia-southeast2-a", "asia-southeast2-b", "asia-southeast2-c"],
  "australia-southeast1": ["australia-southeast1-a", "australia-southeast1-b", "australia-southeast1-c"],
  "australia-southeast2": ["australia-southeast2-a", "australia-southeast2-b", "australia-southeast2-c"],
  "me-west1": ["me-west1-a", "me-west1-b", "me-west1-c"],
  "me-central1": ["me-central1-a", "me-central1-b", "me-central1-c"],
  "africa-south1": ["africa-south1-a", "africa-south1-b", "africa-south1-c"],
};

// Form field definitions based on proto schemas
const formDefinitions: Record<string, FormField[]> = {
  organization: [
    {
      name: "name",
      label: "Organization Name",
      type: "text",
      required: true,
      placeholder: "Enter organization name",
    },
    {
      name: "description",
      label: "Description",
      type: "textarea",
      required: false,
      placeholder: "Enter description (optional)",
    },
  ],
  project: [
    {
      name: "name",
      label: "Project Name",
      type: "text",
      required: true,
      placeholder: "my-project",
    },
    {
      name: "description",
      label: "Description",
      type: "textarea",
      required: false,
      placeholder: "Optional description",
    },
    {
      name: "region",
      label: "GCP Region",
      type: "select",
      required: true,
      options: [
        { value: "us-central1", label: "US Central (Iowa)" },
        { value: "us-east1", label: "US East (South Carolina)" },
        { value: "us-east4", label: "US East (Virginia)" },
        { value: "us-west1", label: "US West (Oregon)" },
        { value: "us-west2", label: "US West (Los Angeles)" },
        { value: "us-west3", label: "US West (Salt Lake City)" },
        { value: "us-west4", label: "US West (Las Vegas)" },
        { value: "us-south1", label: "US South (Dallas)" },
        { value: "northamerica-northeast1", label: "Canada (Montreal)" },
        { value: "northamerica-northeast2", label: "Canada (Toronto)" },
        { value: "southamerica-east1", label: "South America (São Paulo)" },
        { value: "southamerica-west1", label: "South America (Santiago)" },
        { value: "europe-west1", label: "Europe West (Belgium)" },
        { value: "europe-west2", label: "Europe West (London)" },
        { value: "europe-west3", label: "Europe West (Frankfurt)" },
        { value: "europe-west4", label: "Europe West (Netherlands)" },
        { value: "europe-west6", label: "Europe West (Zurich)" },
        { value: "europe-west8", label: "Europe West (Milan)" },
        { value: "europe-west9", label: "Europe West (Paris)" },
        { value: "europe-central2", label: "Europe Central (Warsaw)" },
        { value: "europe-north1", label: "Europe North (Finland)" },
        { value: "europe-southwest1", label: "Europe Southwest (Madrid)" },
        { value: "asia-east1", label: "Asia East (Taiwan)" },
        { value: "asia-east2", label: "Asia East (Hong Kong)" },
        { value: "asia-northeast1", label: "Asia Northeast (Tokyo)" },
        { value: "asia-northeast2", label: "Asia Northeast (Osaka)" },
        { value: "asia-northeast3", label: "Asia Northeast (Seoul)" },
        { value: "asia-south1", label: "Asia South (Mumbai)" },
        { value: "asia-south2", label: "Asia South (Delhi)" },
        { value: "asia-southeast1", label: "Asia Southeast (Singapore)" },
        { value: "asia-southeast2", label: "Asia Southeast (Jakarta)" },
        { value: "australia-southeast1", label: "Australia (Sydney)" },
        { value: "australia-southeast2", label: "Australia (Melbourne)" },
        { value: "me-west1", label: "Middle East (Tel Aviv)" },
        { value: "me-central1", label: "Middle East (Doha)" },
        { value: "africa-south1", label: "Africa (Johannesburg)" },
      ],
    },
    {
      name: "zone",
      label: "GCP Zone",
      type: "select",
      required: true,
      options: [], // Will be populated dynamically based on region
    },
    {
      name: "machine_type",
      label: "Machine Type",
      type: "select",
      required: true,
      options: [
        { value: "e2-micro", label: "e2-micro (0.25-2 vCPU, 1 GB)" },
        { value: "e2-small", label: "e2-small (0.5-2 vCPU, 2 GB)" },
        { value: "e2-medium", label: "e2-medium (1-2 vCPU, 4 GB)" },
        { value: "e2-standard-2", label: "e2-standard-2 (2 vCPU, 8 GB)" },
        { value: "e2-standard-4", label: "e2-standard-4 (4 vCPU, 16 GB)" },
      ],
    },
    {
      name: "disk_size_gb",
      label: "Disk Size (GB)",
      type: "text",
      required: true,
      placeholder: "20",
    },
    {
      name: "create_branch_sites",
      label: "Auto-create sites for new branches",
      type: "checkbox",
      required: false,
    },
  ],
  site: [
    {
      name: "name",
      label: "Site Name",
      type: "text",
      required: true,
      placeholder: "production",
    },
    {
      name: "github_repository",
      label: "GitHub Repository",
      type: "text",
      required: true,
      placeholder: "https://github.com/org/repo",
    },
    {
      name: "github_ref",
      label: "GitHub Ref (branch/tag)",
      type: "text",
      required: true,
      placeholder: "heads/main or tags/v1.0.0",
    },
    {
      name: "compose_path",
      label: "Docker Compose Path",
      type: "text",
      required: false,
      placeholder: "Leave empty for root",
    },
    {
      name: "compose_file",
      label: "Docker Compose File",
      type: "text",
      required: false,
      placeholder: "docker-compose.yml",
    },
    {
      name: "port",
      label: "Application Port",
      type: "text",
      required: false,
      placeholder: "80",
    },
  ],
  firewall: [
    {
      name: "name",
      label: "Rule Name",
      type: "text",
      required: true,
      placeholder: "Enter rule name",
    },
    {
      name: "rule_type",
      label: "Rule Type",
      type: "select",
      required: true,
      options: [
        { value: "1", label: "HTTPS Allowed" },
        { value: "2", label: "SSH Allowed" },
        { value: "3", label: "Blocked" },
      ],
    },
    {
      name: "cidr",
      label: "CIDR Block",
      type: "text",
      required: true,
      placeholder: "203.0.113.0/24",
    },
  ],
  member: [
    {
      name: "account_id",
      label: "Account ID",
      type: "text",
      required: true,
      placeholder: "Enter account ID",
    },
    {
      name: "role",
      label: "Role",
      type: "select",
      required: true,
      options: [
        { value: "read", label: "Read" },
        { value: "developer", label: "Developer" },
        { value: "owner", label: "Owner" },
      ],
    },
  ],
  secret: [
    {
      name: "name",
      label: "Secret Name",
      type: "text",
      required: true,
      placeholder: "e.g., DATABASE_URL",
    },
    {
      name: "value",
      label: "Secret Value",
      type: "textarea",
      required: true,
      placeholder: "Enter the secret value",
    },
  ],
  setting: [
    {
      name: "key",
      label: "Setting Key",
      type: "text",
      required: true,
      placeholder: "e.g., max_upload_size",
    },
    {
      name: "value",
      label: "Setting Value",
      type: "text",
      required: true,
      placeholder: "Enter the setting value",
    },
    {
      name: "description",
      label: "Description",
      type: "textarea",
      required: false,
      placeholder: "Optional description of this setting",
    },
  ],
};

function createFormField(field: FormField): HTMLElement {
  const div = document.createElement("div");
  div.className = "mb-4";

  // Checkbox has different layout
  if (field.type === "checkbox") {
    div.className = "mb-4 flex items-center";

    const input = document.createElement("input");
    input.type = "checkbox";
    input.name = field.name;
    input.id = field.name;
    input.className = "h-4 w-4 text-red-900 focus:ring-red-900 border-gray-300 rounded";
    input.value = "true";
    div.appendChild(input);

    const label = document.createElement("label");
    label.htmlFor = field.name;
    label.className = "ml-2 block text-sm text-gray-700";
    label.textContent = field.label;
    div.appendChild(label);

    return div;
  }

  const label = document.createElement("label");
  label.className = "block text-sm font-medium text-gray-700 mb-2";
  label.textContent = field.label;
  if (field.required) {
    const required = document.createElement("span");
    required.className = "text-red-500";
    required.textContent = " *";
    label.appendChild(required);
  }
  div.appendChild(label);

  let input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement;

  if (field.type === "textarea") {
    input = document.createElement("textarea");
    input.rows = 3;
  } else if (field.type === "select" && field.options) {
    input = document.createElement("select");
    const placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.textContent = `Select ${field.label}`;
    placeholder.disabled = true;
    placeholder.selected = true;
    input.appendChild(placeholder);

    field.options.forEach((opt) => {
      const option = document.createElement("option");
      option.value = opt.value;
      option.textContent = opt.label;
      input.appendChild(option);
    });
  } else {
    input = document.createElement("input");
    input.type = field.type;
  }

  input.name = field.name;
  input.className =
    "w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-red-900";
  input.required = field.required;
  if (field.placeholder) {
    input.placeholder = field.placeholder;
  }

  div.appendChild(input);
  return div;
}

async function buildForm(resourceType: string, onSubmit: (data: Record<string, any>) => void): Promise<HTMLFormElement> {
  const form = document.createElement("form");
  form.className = "space-y-4";

  let fields = formDefinitions[resourceType];
  if (!fields) {
    form.innerHTML = `<div class="text-red-600">No form definition for resource type: ${resourceType}</div>`;
    return form;
  }

  // Add dynamic organization dropdown for projects (only when no org context)
  const context = getPageContext();
  if (resourceType === "project" && !context.organizationId) {
    try {
      const orgsResponse = await organizationClient.listOrganizations({});
      const orgOptions = orgsResponse.organizations.map((org) => ({
        value: org.organizationId,
        label: org.organizationName,
      }));
      fields = [
        {
          name: "organization_id",
          label: "Organization",
          type: "select",
          required: true,
          options: orgOptions,
        },
        ...fields,
      ];
    } catch (error) {
      console.error("Failed to load organizations", error);
    }
  }

  // Add dynamic project dropdown for sites (only when no project context)
  if (resourceType === "site" && !context.projectId) {
    try {
      const projectsResponse = await projectClient.listProjects({});
      const projectOptions = projectsResponse.projects.map((proj) => ({
        value: proj.projectId,
        label: `${proj.projectName} (${proj.organizationId})`,
      }));
      fields = [
        {
          name: "project_id",
          label: "Project",
          type: "select",
          required: true,
          options: projectOptions,
        },
        ...fields,
      ];
    } catch (error) {
      console.error("Failed to load projects", error);
    }
  }

  // Add resource selector for shared resources (firewall, member, secret, setting)
  const sharedResources = ["firewall", "member", "secret", "setting"];
  if (sharedResources.includes(resourceType) && !context.organizationId && !context.projectId && !context.siteId) {
    try {
      const orgsResponse = await organizationClient.listOrganizations({});
      const projectsResponse = await projectClient.listProjects({});

      const resourceOptions = [
        { value: "", label: "Select a resource" },
        ...orgsResponse.organizations.map((org) => ({
          value: `org:${org.organizationId}`,
          label: `Organization: ${org.organizationName}`,
        })),
        ...projectsResponse.projects.map((proj) => ({
          value: `project:${proj.projectId}`,
          label: `Project: ${proj.projectName}`,
        })),
      ];

      fields = [
        {
          name: "resource_selector",
          label: "Attach To",
          type: "select",
          required: true,
          options: resourceOptions,
        },
        ...fields,
      ];
    } catch (error) {
      console.error("Failed to load resources", error);
    }
  }

  fields.forEach((field) => {
    form.appendChild(createFormField(field));
  });

  // Add event listener for region/zone interaction in project forms
  if (resourceType === "project") {
    const regionSelect = form.querySelector('select[name="region"]') as HTMLSelectElement;
    const zoneSelect = form.querySelector('select[name="zone"]') as HTMLSelectElement;

    if (regionSelect && zoneSelect) {
      // Function to update zone options based on selected region
      const updateZoneOptions = (region: string) => {
        // Clear existing options
        zoneSelect.innerHTML = '';

        if (!region) {
          const placeholder = document.createElement("option");
          placeholder.value = "";
          placeholder.textContent = "Select a region first";
          placeholder.disabled = true;
          placeholder.selected = true;
          zoneSelect.appendChild(placeholder);
          return;
        }

        // Add placeholder
        const placeholder = document.createElement("option");
        placeholder.value = "";
        placeholder.textContent = "Select GCP Zone";
        placeholder.disabled = true;
        placeholder.selected = true;
        zoneSelect.appendChild(placeholder);

        // Add zone options for the selected region
        const zones = regionZones[region] || [];
        zones.forEach((zone) => {
          const option = document.createElement("option");
          option.value = zone;
          option.textContent = zone;
          zoneSelect.appendChild(option);
        });
      };

      // Add event listener to update zones when region changes
      regionSelect.addEventListener("change", (e) => {
        const selectedRegion = (e.target as HTMLSelectElement).value;
        updateZoneOptions(selectedRegion);
      });

      // Set initial zone options based on first region (us-central1)
      // This ensures zones are populated when the form first loads
      updateZoneOptions("us-central1");
    }
  }

  const buttonDiv = document.createElement("div");
  buttonDiv.className = "flex justify-end space-x-2 mt-6";

  const cancelButton = document.createElement("button");
  cancelButton.type = "button";
  cancelButton.className =
    "px-4 py-2 bg-gray-300 text-gray-700 rounded-md hover:bg-gray-400";
  cancelButton.textContent = "Cancel";
  cancelButton.onclick = () => closeModal();

  const submitButton = document.createElement("button");
  submitButton.type = "submit";
  submitButton.className =
    "px-4 py-2 bg-red-900 text-white rounded-md hover:bg-red-950";
  submitButton.textContent = "Save";

  buttonDiv.appendChild(cancelButton);
  buttonDiv.appendChild(submitButton);
  form.appendChild(buttonDiv);

  form.onsubmit = async (e) => {
    e.preventDefault();
    const formData = new FormData(form);
    const data: Record<string, any> = {};
    formData.forEach((value, key) => {
      data[key] = value;
    });

    // Handle checkboxes - if not in formData, they are false
    fields.forEach((field) => {
      if (field.type === "checkbox" && !data[field.name]) {
        data[field.name] = false;
      } else if (field.type === "checkbox" && data[field.name] === "true") {
        data[field.name] = true;
      }
    });

    onSubmit(data);
  };

  return form;
}

export async function openCreateModal(resourceType: string) {
  const context = getPageContext();
  const singularType = singularize(resourceType);

  showLoadingModal(`Create ${capitalize(singularType)}`);

  setTimeout(async () => {
    const form = await buildForm(singularType, async (data) => {
      // Handle resource selector for shared resources
      if (data.resource_selector) {
        const [type, id] = data.resource_selector.split(":");
        if (type === "org") {
          data.organizationId = id;
        } else if (type === "project") {
          data.projectId = id;
        } else if (type === "site") {
          data.siteId = id;
        }
        delete data.resource_selector;
      }

      // Handle organization_id from dropdown
      if (data.organization_id) {
        data.organizationId = data.organization_id;
        delete data.organization_id;
      }

      // Handle project_id from dropdown
      if (data.project_id) {
        const projectsResponse = await projectClient.listProjects({});
        const selectedProject = projectsResponse.projects.find(p => p.projectId === data.project_id);
        if (selectedProject) {
          data.projectId = data.project_id;
          data.organizationId = selectedProject.organizationId;
        }
        delete data.project_id;
      }
      switch (singularType) {
        case "organization":
          await createOrganization(data);
          break;
        case "project":
          if (!data.organizationId && !context.organizationId) {
            showNotification("error", "Organization ID not found");
            return;
          }
          await createProject({
            organizationId: data.organizationId || context.organizationId,
            ...data,
          });
          break;
        case "site":
          if (!data.organizationId && !context.organizationId) {
            showNotification("error", "Organization ID not found");
            return;
          }
          if (!data.projectId && !context.projectId) {
            showNotification("error", "Project ID not found");
            return;
          }
          await createSite({
            organizationId: data.organizationId || context.organizationId,
            projectId: data.projectId || context.projectId,
            ...data,
          });
          break;
        case "firewall":
          await createFirewallRule({
            organizationId: data.organizationId || context.organizationId || undefined,
            projectId: data.projectId || context.projectId || undefined,
            siteId: data.siteId || context.siteId || undefined,
            ruleType: parseInt(data.rule_type),
            cidr: data.cidr,
            name: data.name,
          });
          break;
        case "member":
          await createMember({
            organizationId: data.organizationId || context.organizationId || undefined,
            projectId: data.projectId || context.projectId || undefined,
            siteId: data.siteId || context.siteId || undefined,
            accountId: data.account_id,
            role: data.role,
          });
          break;
        case "secret":
          await createSecret({
            organizationId: data.organizationId || context.organizationId || undefined,
            projectId: data.projectId || context.projectId || undefined,
            siteId: data.siteId || context.siteId || undefined,
            name: data.name,
            value: data.value,
          });
          break;
        case "setting":
          await createSetting({
            organizationId: data.organizationId || context.organizationId || undefined,
            projectId: data.projectId || context.projectId || undefined,
            siteId: data.siteId || context.siteId || undefined,
            key: data.key,
            value: data.value,
            description: data.description || "",
            editable: true,
          });
          break;
        default:
          showNotification("error", `Unknown resource type: ${singularType}`);
      }
    });

    const modal = document.getElementById("modal");
    const titleElement = document.getElementById("modal-title");
    const contentElement = document.getElementById("modal-content");

    if (modal && titleElement && contentElement) {
      titleElement.textContent = `Create ${capitalize(singularType)}`;
      contentElement.innerHTML = "";
      contentElement.appendChild(form);
    }
  }, 100);
}

export function openEditModal(resourceType: string, resourceId: string) {
  const context = getPageContext();
  const singularType = singularize(resourceType);

  showLoadingModal(`Edit ${capitalize(singularType)}`);

  // In a real implementation, you would fetch the current data here
  // For now, we'll just show an empty form
  setTimeout(() => {
    const form = buildForm(singularType, async (data) => {
      switch (singularType) {
        case "organization":
          await updateOrganization(resourceId, data);
          break;
        case "project":
          if (!context.organizationId) {
            showNotification("error", "Organization ID not found");
            return;
          }
          await updateProject(context.organizationId, resourceId, data);
          break;
        case "site":
          await updateSite(resourceId, data);
          break;
        case "member":
          await updateMember({
            organizationId: context.organizationId || undefined,
            projectId: context.projectId || undefined,
            siteId: context.siteId || undefined,
            accountId: resourceId,
            role: data.role,
          });
          break;
        case "secret":
          await updateSecret({
            organizationId: context.organizationId || undefined,
            projectId: context.projectId || undefined,
            siteId: context.siteId || undefined,
            secretId: resourceId,
            name: data.name,
            value: data.value,
          });
          break;
        case "setting":
          await updateSetting({
            organizationId: context.organizationId || undefined,
            projectId: context.projectId || undefined,
            siteId: context.siteId || undefined,
            settingId: resourceId,
            key: data.key,
            value: data.value,
            description: data.description || "",
            editable: true,
          });
          break;
        default:
          showNotification("error", `Unknown resource type: ${singularType}`);
      }
    });

    const modal = document.getElementById("modal");
    const titleElement = document.getElementById("modal-title");
    const contentElement = document.getElementById("modal-content");

    if (modal && titleElement && contentElement) {
      titleElement.textContent = `Edit ${capitalize(singularType)}`;
      contentElement.innerHTML = "";
      contentElement.appendChild(form);
    }
  }, 100);
}
