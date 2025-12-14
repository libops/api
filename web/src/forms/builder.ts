// Proto-based form builder

import { getPageContext } from "@/utils/context";
import { showLoadingModal, closeModal } from "@/utils/modal";
import { showNotification, capitalize, singularize } from "@/utils/helpers";
import {
  createOrganization,
  createProject,
  createSite,
  createFirewallRule,
  createMember,
  updateOrganization,
  updateProject,
  updateSite,
  updateMember,
} from "@/resources/operations";

interface FormField {
  name: string;
  label: string;
  type: string;
  required: boolean;
  placeholder?: string;
  options?: { value: string; label: string }[];
}

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
      placeholder: "Enter project name",
    },
    {
      name: "description",
      label: "Description",
      type: "textarea",
      required: false,
      placeholder: "Enter description (optional)",
    },
  ],
  site: [
    {
      name: "name",
      label: "Site Name",
      type: "text",
      required: true,
      placeholder: "Enter site name",
    },
    {
      name: "git_repo_url",
      label: "Git Repository URL",
      type: "text",
      required: false,
      placeholder: "https://github.com/org/repo",
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
};

function createFormField(field: FormField): HTMLElement {
  const div = document.createElement("div");
  div.className = "mb-4";

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
    "w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-green-500";
  input.required = field.required;
  if (field.placeholder) {
    input.placeholder = field.placeholder;
  }

  div.appendChild(input);
  return div;
}

function buildForm(resourceType: string, onSubmit: (data: Record<string, any>) => void): HTMLFormElement {
  const form = document.createElement("form");
  form.className = "space-y-4";

  const fields = formDefinitions[resourceType];
  if (!fields) {
    form.innerHTML = `<div class="text-red-600">No form definition for resource type: ${resourceType}</div>`;
    return form;
  }

  fields.forEach((field) => {
    form.appendChild(createFormField(field));
  });

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
    "px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700";
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
    onSubmit(data);
  };

  return form;
}

export function openCreateModal(resourceType: string) {
  const context = getPageContext();
  const singularType = singularize(resourceType);

  showLoadingModal(`Create ${capitalize(singularType)}`);

  setTimeout(() => {
    const form = buildForm(singularType, async (data) => {
      switch (singularType) {
        case "organization":
          await createOrganization(data);
          break;
        case "project":
          if (!context.organizationId) {
            showNotification("error", "Organization ID not found");
            return;
          }
          await createProject({
            organizationId: context.organizationId,
            ...data,
          });
          break;
        case "site":
          if (!context.organizationId || !context.projectId) {
            showNotification("error", "Organization or Project ID not found");
            return;
          }
          await createSite({
            organizationId: context.organizationId,
            projectId: context.projectId,
            ...data,
          });
          break;
        case "firewall":
          await createFirewallRule({
            organizationId: context.organizationId || undefined,
            projectId: context.projectId || undefined,
            siteId: context.siteId || undefined,
            ruleType: parseInt(data.rule_type),
            cidr: data.cidr,
            name: data.name,
          });
          break;
        case "member":
          await createMember({
            organizationId: context.organizationId || undefined,
            projectId: context.projectId || undefined,
            siteId: context.siteId || undefined,
            accountId: data.account_id,
            role: data.role,
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
