// Resource CRUD operations using ConnectRPC

import {
  organizationClient,
  projectClient,
  siteClient,
  firewallClient,
  projectFirewallClient,
  siteFirewallClient,
  memberClient,
  projectMemberClient,
  siteMemberClient,
} from "@/api/client";
import { getPageContext } from "@/utils/context";
import { showNotification, capitalize, singularize } from "@/utils/helpers";
import { closeModal } from "@/utils/modal";

// Organization operations
export async function createOrganization(data: { name: string; description?: string }) {
  try {
    const response = await organizationClient.createOrganization({
      folder: {
        name: data.name,
        description: data.description || "",
      },
    });
    showNotification("success", "Organization created successfully");
    closeModal();
    window.location.reload();
    return response;
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function updateOrganization(
  organizationId: string,
  data: { name?: string; description?: string }
) {
  try {
    const response = await organizationClient.updateOrganization({
      organizationId,
      folder: {
        name: data.name || "",
        description: data.description || "",
      },
    });
    showNotification("success", "Organization updated successfully");
    closeModal();
    window.location.reload();
    return response;
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function deleteOrganization(organizationId: string) {
  if (!confirm("Are you sure you want to delete this organization?")) {
    return;
  }

  try {
    await organizationClient.deleteOrganization({ organizationId });
    showNotification("success", "Organization deleted successfully");
    window.location.reload();
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

// Project operations
export async function createProject(data: {
  organizationId: string;
  name: string;
  description?: string;
}) {
  try {
    const response = await projectClient.createProject({
      organizationId: data.organizationId,
      project: {
        name: data.name,
        description: data.description || "",
      },
    });
    showNotification("success", "Project created successfully");
    closeModal();
    window.location.reload();
    return response;
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function updateProject(
  organizationId: string,
  projectId: string,
  data: { name?: string; description?: string }
) {
  try {
    const response = await projectClient.updateProject({
      organizationId,
      projectId,
      project: {
        name: data.name || "",
        description: data.description || "",
      },
    });
    showNotification("success", "Project updated successfully");
    closeModal();
    window.location.reload();
    return response;
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function deleteProject(organizationId: string, projectId: string) {
  if (!confirm("Are you sure you want to delete this project?")) {
    return;
  }

  try {
    await projectClient.deleteProject({ organizationId, projectId });
    showNotification("success", "Project deleted successfully");
    window.location.reload();
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

// Site operations
export async function createSite(data: {
  organizationId: string;
  projectId: string;
  name: string;
  gitRepoUrl?: string;
}) {
  try {
    const response = await siteClient.createSite({
      organizationId: data.organizationId,
      projectId: data.projectId,
      site: {
        name: data.name,
        gitRepoUrl: data.gitRepoUrl || "",
      },
    });
    showNotification("success", "Site created successfully");
    closeModal();
    window.location.reload();
    return response;
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function updateSite(
  siteId: string,
  data: { name?: string; gitRepoUrl?: string }
) {
  try {
    const response = await siteClient.updateSite({
      siteId,
      site: {
        name: data.name || "",
        gitRepoUrl: data.gitRepoUrl || "",
      },
    });
    showNotification("success", "Site updated successfully");
    closeModal();
    window.location.reload();
    return response;
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function deleteSite(siteId: string) {
  if (!confirm("Are you sure you want to delete this site?")) {
    return;
  }

  try {
    await siteClient.deleteSite({ siteId });
    showNotification("success", "Site deleted successfully");
    window.location.reload();
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

// Firewall operations
export async function createFirewallRule(data: {
  organizationId?: string;
  projectId?: string;
  siteId?: string;
  ruleType: number;
  cidr: string;
  name: string;
}) {
  try {
    if (data.siteId) {
      const response = await siteFirewallClient.createSiteFirewallRule({
        siteId: data.siteId,
        ruleType: data.ruleType,
        cidr: data.cidr,
        name: data.name,
      });
      showNotification("success", "Firewall rule created successfully");
      closeModal();
      window.location.reload();
      return response;
    } else if (data.projectId) {
      const response = await projectFirewallClient.createProjectFirewallRule({
        projectId: data.projectId,
        ruleType: data.ruleType,
        cidr: data.cidr,
        name: data.name,
      });
      showNotification("success", "Firewall rule created successfully");
      closeModal();
      window.location.reload();
      return response;
    } else if (data.organizationId) {
      const response = await firewallClient.createOrganizationFirewallRule({
        organizationId: data.organizationId,
        ruleType: data.ruleType,
        cidr: data.cidr,
        name: data.name,
      });
      showNotification("success", "Firewall rule created successfully");
      closeModal();
      window.location.reload();
      return response;
    }
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function deleteFirewallRule(ruleId: string) {
  if (!confirm("Are you sure you want to delete this firewall rule?")) {
    return;
  }

  const context = getPageContext();

  try {
    if (context.siteId) {
      await siteFirewallClient.deleteSiteFirewallRule({
        siteId: context.siteId,
        ruleId,
      });
    } else if (context.projectId) {
      await projectFirewallClient.deleteProjectFirewallRule({
        projectId: context.projectId,
        ruleId,
      });
    } else if (context.organizationId) {
      await firewallClient.deleteOrganizationFirewallRule({
        organizationId: context.organizationId,
        ruleId,
      });
    }
    showNotification("success", "Firewall rule deleted successfully");
    window.location.reload();
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

// Member operations
export async function createMember(data: {
  organizationId?: string;
  projectId?: string;
  siteId?: string;
  accountId: string;
  role: string;
}) {
  try {
    if (data.siteId) {
      const response = await siteMemberClient.createSiteMember({
        siteId: data.siteId,
        accountId: data.accountId,
        role: data.role,
      });
      showNotification("success", "Member added successfully");
      closeModal();
      window.location.reload();
      return response;
    } else if (data.projectId) {
      const response = await projectMemberClient.createProjectMember({
        projectId: data.projectId,
        accountId: data.accountId,
        role: data.role,
      });
      showNotification("success", "Member added successfully");
      closeModal();
      window.location.reload();
      return response;
    } else if (data.organizationId) {
      const response = await memberClient.createOrganizationMember({
        organizationId: data.organizationId,
        accountId: data.accountId,
        role: data.role,
      });
      showNotification("success", "Member added successfully");
      closeModal();
      window.location.reload();
      return response;
    }
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function updateMember(data: {
  organizationId?: string;
  projectId?: string;
  siteId?: string;
  accountId: string;
  role: string;
}) {
  try {
    if (data.siteId) {
      const response = await siteMemberClient.updateSiteMember({
        siteId: data.siteId,
        accountId: data.accountId,
        role: data.role,
      });
      showNotification("success", "Member updated successfully");
      closeModal();
      window.location.reload();
      return response;
    } else if (data.projectId) {
      const response = await projectMemberClient.updateProjectMember({
        projectId: data.projectId,
        accountId: data.accountId,
        role: data.role,
      });
      showNotification("success", "Member updated successfully");
      closeModal();
      window.location.reload();
      return response;
    } else if (data.organizationId) {
      const response = await memberClient.updateOrganizationMember({
        organizationId: data.organizationId,
        accountId: data.accountId,
        role: data.role,
      });
      showNotification("success", "Member updated successfully");
      closeModal();
      window.location.reload();
      return response;
    }
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

export async function deleteMember(accountId: string) {
  if (!confirm("Are you sure you want to remove this member?")) {
    return;
  }

  const context = getPageContext();

  try {
    if (context.siteId) {
      await siteMemberClient.deleteSiteMember({
        siteId: context.siteId,
        accountId,
      });
    } else if (context.projectId) {
      await projectMemberClient.deleteProjectMember({
        projectId: context.projectId,
        accountId,
      });
    } else if (context.organizationId) {
      await memberClient.deleteOrganizationMember({
        organizationId: context.organizationId,
        accountId,
      });
    }
    showNotification("success", "Member removed successfully");
    window.location.reload();
  } catch (error) {
    showNotification("error", (error as Error).message);
    throw error;
  }
}

// Generic delete resource function
export async function deleteResource(resourceType: string, resourceId: string) {
  const context = getPageContext();

  switch (singularize(resourceType)) {
    case "organization":
      return deleteOrganization(resourceId);
    case "project":
      if (!context.organizationId) {
        showNotification("error", "Organization ID not found");
        return;
      }
      return deleteProject(context.organizationId, resourceId);
    case "site":
      return deleteSite(resourceId);
    case "firewall":
      return deleteFirewallRule(resourceId);
    case "member":
      return deleteMember(resourceId);
    default:
      showNotification("error", `Unknown resource type: ${resourceType}`);
  }
}
