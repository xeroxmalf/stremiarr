# Security Policy

## Supported Versions
Security updates are currently only provided for the latest release on the `main` branch.

## Reporting a Vulnerability
If you discover a security vulnerability in Stremiarr, please report it immediately.
- Open a **private issue** on GitHub, or
- Send an email to the security team (if an email is provided by the repository owner).

We treat all security reports with the highest priority.

## Security Measures in the Project
We actively maintain the security of our project using the following tools and practices:
- **Trivy scanning**: Automated container and dependency vulnerability scanning.
- **Shellcheck**: Static analysis for shell scripts to prevent common shell vulnerabilities.
- **validate_security.sh**: A dedicated script to validate security constraints across the repository.

## Secrets Management
- **.env files**: All secrets, passwords, and API keys must be managed through `.env` files.
- **Never commit secrets**: Ensure that `.env` and any other files containing sensitive information are added to `.gitignore` and never committed to the repository.

## Response Timeline Expectations
- We aim to acknowledge receipt of vulnerability reports within 48 hours.
- We strive to provide a preliminary assessment and estimated timeline for a fix within 7 days.
- Security patches will be prioritized over feature development.
