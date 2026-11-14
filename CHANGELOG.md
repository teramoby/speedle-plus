# Release notes

## Unreleased

- Removed the experimental policy-distribution feature introduced in v1.7.x.
  The Engine continues to provide policy decisions, local policy stores,
  management and decision APIs, spctl and SDK integration. Configurations that
  select the removed store type fail at startup with a v1.8.0 migration error.
