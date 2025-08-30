### What
Auto mitigation by OPS Agent

### Why
- Elevated latency / errors detected for the target service
- Action: toggle feature flag off

### Checks
- [ ] ArgoCD shows **Synced** and **Healthy**
- [ ] 5-min avg latency improves ≥ 10% vs previous window
