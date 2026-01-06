---
title: Operational Procedures
sidebar_label: Operational Procedures
sidebar_position: 9
---

# Operational Procedures

Checklists and procedures for operational phases.

## Cleanup

### Uninstall Service

```bash
# Linux
sudo muti-metroo service uninstall

# macOS
sudo muti-metroo service uninstall

# Windows (as Administrator)
muti-metroo.exe service uninstall
```

### Remove Artifacts

```bash
# Remove data directory
rm -rf /path/to/data_dir

# Remove binary and config
rm /path/to/muti-metroo
rm /path/to/config.yaml

# Clear relevant logs
# (location depends on configuration)
```

### Forensic Considerations

- Agent ID is stored in `data_dir/identity.json`
- E2E keypair stored in `data_dir/keypair.json`
- No persistent logs by default (when `log_level: error`)
- Memory contains active session keys

## Operational Checklist

### Pre-Deployment

- [ ] Generate management keypair (keep private key secure)
- [ ] Generate unique agent certificates
- [ ] Configure stealth protocol settings (empty identifiers)
- [ ] Set appropriate log level (`error` or `warn`)
- [ ] Prepare environment-specific config files
- [ ] Test connectivity through target network path

### Deployment

- [ ] Transfer binary (renamed appropriately)
- [ ] Deploy config with environment variables
- [ ] Initialize agent identity
- [ ] Install as service (if persistence needed)
- [ ] Verify connectivity to mesh
- [ ] Test shell and file transfer capabilities

### Post-Operation

- [ ] Uninstall services on all agents
- [ ] Remove binaries and configs
- [ ] Clear data directories
- [ ] Document accessed systems
- [ ] Verify cleanup completeness

## Legal and Ethical Considerations

- Always obtain written authorization before deployment
- Document all activities for engagement report
- Respect scope boundaries strictly
- Report unexpected findings through proper channels
- Coordinate with blue team per rules of engagement
- Retain evidence per engagement requirements
