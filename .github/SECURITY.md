# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| >= 0.1.x| :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please **DO NOT** open a public issue.

**How to Report:** Use GitHub's [Private Vulnerability Reporting](https://github.com/gravitino/terraform-provider-gravitino/security/advisories/new)

**What to Include:**
- Affected versions
- Impact (data exposure, privilege escalation, etc.)
- Reproduction steps
- Proof of concept if possible

**Response Timeline:**
- Initial response: within 48 hours
- Assessment: within 7 days
- Fix release: typically within 14 days

## Security Best Practices for Users

1. Use HTTPS connections to Gravitino servers
2. Never hardcode credentials in Terraform files
3. Encrypt Terraform state files
4. Use least-privilege authentication
5. Rotate credentials regularly
