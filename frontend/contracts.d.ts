export type Role = "ORG_ADMIN" | "ASSESSOR" | "VIEWER";
export type OrganizationState =
  | "DRAFT"
  | "SUBMITTED"
  | "UNDER_REVIEW"
  | "CHANGES_REQUESTED"
  | "REJECTED"
  | "APPROVED"
  | "PROVISIONING"
  | "PROVISIONING_FAILED"
  | "ACTIVE"
  | "SUSPENDED"
  | "OFFBOARDING"
  | "OFFBOARDING_FAILED"
  | "OFFBOARDED";
export interface Membership {
  orgId: string;
  role: Role;
  active: boolean;
  displayName: string;
  state: OrganizationState;
}
export interface Identity {
  sub: string;
  email: string;
  company: boolean;
  memberships: Membership[];
  teamAccount?: boolean;
}
export interface Upload {
  url: string;
  method: "PUT";
  headers: Record<string, string>;
  expiresInSeconds: number;
}
export interface Assessment {
  sk: string;
  studentId: string;
  recordingId: string;
  state: "AWAITING_UPLOAD" | "QUEUED" | "PROCESSING" | "COMPLETED" | "FAILED";
  upload?: Upload;
  error?: string;
  result?: {
    transcript: string;
    score: {
      accuracy_score: number;
      pronunciation_score: number;
      pause_count: number | null;
    };
    wordsPerMinute: number;
  };
}

export interface AssessmentProcessingSettings {
  integrationsEnabled: boolean;
  enabled: boolean;
  platformAvailable: boolean;
  effectiveEnabled: boolean;
}

export interface OrganizationIntegrationSettings {
  enabled: boolean;
  inherited: boolean;
  assessmentConfigured: boolean;
  emailConfigured: boolean;
  assessmentEnabled: boolean;
  emailEnabled: boolean;
}
