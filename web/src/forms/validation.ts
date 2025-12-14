// Form validation utilities

export interface ValidationRule {
  validate: (value: string) => boolean;
  message: string;
}

export interface FieldValidation {
  required?: boolean;
  minLength?: number;
  maxLength?: number;
  pattern?: RegExp;
  custom?: ValidationRule[];
}

/**
 * Common validation rules
 */
export const ValidationRules = {
  email: {
    validate: (value: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value),
    message: "Please enter a valid email address",
  },
  cidr: {
    validate: (value: string) => {
      // Basic CIDR validation (e.g., 192.168.1.0/24)
      return /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/.test(value);
    },
    message: "Please enter a valid CIDR block (e.g., 192.168.1.0/24)",
  },
  url: {
    validate: (value: string) => {
      try {
        new URL(value);
        return true;
      } catch {
        return false;
      }
    },
    message: "Please enter a valid URL",
  },
  noSpaces: {
    validate: (value: string) => !/\s/.test(value),
    message: "Spaces are not allowed",
  },
  alphanumeric: {
    validate: (value: string) => /^[a-zA-Z0-9_-]+$/.test(value),
    message: "Only letters, numbers, hyphens, and underscores are allowed",
  },
};

/**
 * Validate a single field
 */
export function validateField(
  value: string,
  validation: FieldValidation
): { valid: boolean; error?: string } {
  // Required check
  if (validation.required && !value.trim()) {
    return { valid: false, error: "This field is required" };
  }

  // Skip other validations if empty and not required
  if (!value.trim()) {
    return { valid: true };
  }

  // Min length
  if (validation.minLength && value.length < validation.minLength) {
    return {
      valid: false,
      error: `Must be at least ${validation.minLength} characters`,
    };
  }

  // Max length
  if (validation.maxLength && value.length > validation.maxLength) {
    return {
      valid: false,
      error: `Must be at most ${validation.maxLength} characters`,
    };
  }

  // Pattern
  if (validation.pattern && !validation.pattern.test(value)) {
    return { valid: false, error: "Invalid format" };
  }

  // Custom rules
  if (validation.custom) {
    for (const rule of validation.custom) {
      if (!rule.validate(value)) {
        return { valid: false, error: rule.message };
      }
    }
  }

  return { valid: true };
}

/**
 * Add validation to a form element
 */
export function addFieldValidation(
  input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
  validation: FieldValidation
) {
  const errorElement = document.createElement("div");
  errorElement.className = "text-red-600 text-sm mt-1 hidden";
  errorElement.setAttribute("role", "alert");
  input.parentElement?.appendChild(errorElement);

  const validate = () => {
    const result = validateField(input.value, validation);

    if (!result.valid && result.error) {
      input.classList.add("border-red-500", "focus:ring-red-500", "focus:border-red-500");
      input.classList.remove("border-gray-300", "focus:ring-green-500", "focus:border-green-500");
      errorElement.textContent = result.error;
      errorElement.classList.remove("hidden");
      return false;
    } else {
      input.classList.remove("border-red-500", "focus:ring-red-500", "focus:border-red-500");
      input.classList.add("border-gray-300", "focus:ring-green-500", "focus:border-green-500");
      errorElement.classList.add("hidden");
      return true;
    }
  };

  // Validate on blur
  input.addEventListener("blur", validate);

  // Validate on input (after first blur)
  let hasBlurred = false;
  input.addEventListener("blur", () => {
    hasBlurred = true;
  }, { once: true });

  input.addEventListener("input", () => {
    if (hasBlurred) {
      validate();
    }
  });

  return validate;
}

/**
 * Validate an entire form
 */
export function validateForm(form: HTMLFormElement): boolean {
  const inputs = form.querySelectorAll<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(
    "input, textarea, select"
  );

  let isValid = true;

  inputs.forEach((input) => {
    // Trigger native HTML5 validation
    if (!input.checkValidity()) {
      isValid = false;
      input.reportValidity();
    }
  });

  return isValid;
}

/**
 * Resource-specific validation rules
 */
export const ResourceValidations: Record<string, Record<string, FieldValidation>> = {
  organization: {
    name: {
      required: true,
      minLength: 3,
      maxLength: 50,
      custom: [ValidationRules.alphanumeric],
    },
  },
  project: {
    name: {
      required: true,
      minLength: 3,
      maxLength: 50,
      custom: [ValidationRules.alphanumeric],
    },
    git_repo_url: {
      custom: [ValidationRules.url],
    },
  },
  site: {
    name: {
      required: true,
      minLength: 3,
      maxLength: 50,
      custom: [ValidationRules.alphanumeric],
    },
  },
  firewall: {
    name: {
      required: true,
      minLength: 3,
      maxLength: 50,
    },
    cidr: {
      required: true,
      custom: [ValidationRules.cidr],
    },
  },
  member: {
    account_id: {
      required: true,
      custom: [ValidationRules.email],
    },
  },
};
