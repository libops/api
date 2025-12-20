import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { OrganizationService } from "@proto/libops/v1/organization_api_connect";
import { ProjectService } from "@proto/libops/v1/organization_api_connect";
import { SiteService } from "@proto/libops/v1/organization_api_connect";
import { FirewallService } from "@proto/libops/v1/organization_api_connect";
import { ProjectFirewallService } from "@proto/libops/v1/organization_api_connect";
import { SiteFirewallService } from "@proto/libops/v1/organization_api_connect";
import { MemberService } from "@proto/libops/v1/organization_api_connect";
import { ProjectMemberService } from "@proto/libops/v1/organization_api_connect";
import { SiteMemberService } from "@proto/libops/v1/organization_api_connect";
import { SshKeyService } from "@proto/libops/v1/organization_api_connect";
import { AccountService } from "@proto/libops/v1/organization_account_api_connect";
import { OrganizationSecretService, ProjectSecretService, SiteSecretService } from "@proto/libops/v1/secrets_connect";
import { OrganizationSettingService, ProjectSettingService, SiteSettingService } from "@proto/libops/v1/settings_connect";
import { errorInterceptor, loggingInterceptor, loadingInterceptor, retryInterceptor } from "./interceptors";

// Determine if we're in development mode (defaults to production)
// Set window.LIBOPS_ENV = 'development' to enable dev mode
const isDevelopment = (window as any).LIBOPS_ENV === 'development';

// Configure the transport (how we talk to the server)
const transport = createConnectTransport({
  baseUrl: window.location.origin,
  // Add interceptors for retry, loading, error handling, and logging
  // Order matters: retry → loading → error → logging
  interceptors: [
    retryInterceptor,
    loadingInterceptor,
    errorInterceptor,
    ...(isDevelopment ? [loggingInterceptor] : []),
  ],
});

// Create typed clients for each service
export const organizationClient = createPromiseClient(OrganizationService, transport);

export const projectClient = createPromiseClient(ProjectService, transport);

export const siteClient = createPromiseClient(SiteService, transport);

export const firewallClient = createPromiseClient(FirewallService, transport);

export const projectFirewallClient = createPromiseClient(ProjectFirewallService, transport);

export const siteFirewallClient = createPromiseClient(SiteFirewallService, transport);

export const memberClient = createPromiseClient(MemberService, transport);

export const projectMemberClient = createPromiseClient(ProjectMemberService, transport);

export const siteMemberClient = createPromiseClient(SiteMemberService, transport);

export const sshKeyClient = createPromiseClient(SshKeyService, transport);

export const accountClient = createPromiseClient(AccountService, transport);

export const organizationSecretClient = createPromiseClient(OrganizationSecretService, transport);

export const projectSecretClient = createPromiseClient(ProjectSecretService, transport);

export const siteSecretClient = createPromiseClient(SiteSecretService, transport);

export const organizationSettingClient = createPromiseClient(OrganizationSettingService, transport);

export const projectSettingClient = createPromiseClient(ProjectSettingService, transport);

export const siteSettingClient = createPromiseClient(SiteSettingService, transport);
