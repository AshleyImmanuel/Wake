---
description: Enforce strict adherence to the Wake PRD and architectural vision.
---

# Wake PRD Alignment

When working in this project:

1. **Understand the Architecture**: Wake is a local, zero-config state recovery engine and MCP wrapper designed for developers and AI agents. It is NOT a multi-tenant, public-facing web application.
2. **Contextual Security**: Do not hallucinate web-based security vulnerabilities (like traditional server-side request forgery, remote log poisoning, or multi-user state manipulation) for local developer tools where the user/agent already has full system access.
3. **Vision Adherence**: Always cross-reference your architectural decisions with `DOCUMENTATION.md` and `README.md`.
4. **Do Not Overcomplicate**: Keep the footprint small and token-efficient. If a requested feature simulates data or creates unnecessary mock abstractions, challenge the necessity of it before implementing. Wake must remain a lean, production-ready binary.
