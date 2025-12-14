// Page context utilities

export interface PageContext {
  resourceType: string | null;
  resourceId: string | null;
  organizationId: string | null;
  projectId: string | null;
  siteId: string | null;
}

export function getPageContext(): PageContext {
  const context: PageContext = {
    resourceType: null,
    resourceId: null,
    organizationId: null,
    projectId: null,
    siteId: null,
  };

  const body = document.body;

  if (body.dataset.resourceType) {
    context.resourceType = body.dataset.resourceType;
  }
  if (body.dataset.resourceId) {
    context.resourceId = body.dataset.resourceId;
  }
  if (body.dataset.organizationId) {
    context.organizationId = body.dataset.organizationId;
  }
  if (body.dataset.projectId) {
    context.projectId = body.dataset.projectId;
  }
  if (body.dataset.siteId) {
    context.siteId = body.dataset.siteId;
  }

  // Fallback: try to extract from URL path
  const pathParts = window.location.pathname.split("/").filter((p) => p);

  if (pathParts.length >= 2) {
    const potentialType = pathParts[0];
    const potentialId = pathParts[1];

    if (
      !context.resourceType &&
      ["organizations", "projects", "sites"].includes(potentialType)
    ) {
      context.resourceType = potentialType.slice(0, -1); // singular form
      context.resourceId = potentialId;

      if (potentialType === "organizations") {
        context.organizationId = potentialId;
      } else if (potentialType === "projects") {
        context.projectId = potentialId;
      } else if (potentialType === "sites") {
        context.siteId = potentialId;
      }
    }
  }

  return context;
}
