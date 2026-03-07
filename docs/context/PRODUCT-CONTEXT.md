# PRODUCT CONTEXT

## Problem
Agents need secrets for autonomous workflows, but direct secret mounts increase exfiltration risk under prompt injection.

## Product intent
Provide human-approved, time-bound, least-privilege secret leases to keep workflows usable and safer.

## Primary users
- Human operator approving/denying access
- Agent requesting scoped secrets for bounded time

## Core trust boundary
- Secret authority and audit trail are outside agent-controlled workspace.
