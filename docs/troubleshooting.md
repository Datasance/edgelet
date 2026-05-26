# Troubleshooting Guide

Common issues and solutions for the ioFog Agent.

## General Issues

### Agent Won't Start

**Symptoms:**
- Service fails to start
- Error messages in logs

**Solutions:**

1. Check configuration:
   ```bash
   sudo iofog-agent config validate
   ```

2. Check logs:
   ```bash
   sudo journalctl -u iofog-agent -n 50
   ```

3. Verify Docker is running:
   ```bash
   docker ps
   ```

4. Check permissions:
   ```bash
   ls -la /etc/iofog-agent/
   ls -la /var/lib/iofog-agent/
   ```

### Agent Crashes

**Symptoms:**
- Agent stops unexpectedly
- Service restarts repeatedly

**Solutions:**

1. Check system resources:
   ```bash
   free -h
   df -h
   ```

2. Review crash logs:
   ```bash
   sudo journalctl -u iofog-agent --since "1 hour ago"
   ```

3. Check for OOM kills:
   ```bash
   dmesg | grep -i oom
   ```

4. Increase resource limits if needed

## Docker Issues

### Cannot Connect to Docker

**Symptoms:**
- "Cannot connect to Docker daemon" errors
- Containers not starting

**Solutions:**

1. Verify Docker is running:
   ```bash
   sudo systemctl status docker
   ```

2. Check Docker socket permissions:
   ```bash
   ls -la /var/run/docker.sock
   ```

3. Add user to docker group (if not running as root):
   ```bash
   sudo usermod -aG docker $USER
   ```

4. Restart Docker:
   ```bash
   sudo systemctl restart docker
   ```

### Container Creation Fails

**Symptoms:**
- Microservices not starting
- Container creation errors

**Solutions:**

1. Check Docker logs:
   ```bash
   docker logs <container-id>
   ```

2. Verify image exists:
   ```bash
   docker images
   ```

3. Check disk space:
   ```bash
   df -h
   ```

4. Clean up Docker:
   ```bash
   docker system prune -a
   ```

## Network Issues

### Cannot Connect to Controller

**Symptoms:**
- Field Agent not connecting
- Configuration sync failures

**Solutions:**

1. Test connectivity:
   ```bash
   curl -v https://controller.example.com/api/v1/status
   ```

2. Check DNS resolution:
   ```bash
   nslookup controller.example.com
   ```

3. Verify firewall rules:
   ```bash
   sudo iptables -L
   ```

4. Check proxy settings if behind proxy

5. Verify certificates:
   ```bash
   openssl s_client -connect controller.example.com:443
   ```

### Local API Not Accessible

**Symptoms:**
- Cannot access Local API
- Connection refused errors

**Solutions:**

1. Check if agent is running:
   ```bash
   sudo systemctl status iofog-agent
   ```

2. Verify port is listening:
   ```bash
   sudo netstat -tlnp | grep 54321
   ```

3. Check firewall:
   ```bash
   sudo ufw status
   ```

4. Test locally:
   ```bash
   curl http://localhost:54321/api/v1/status
   ```

## Configuration Issues

### Invalid Configuration

**Symptoms:**
- Configuration validation fails
- Agent won't start with config

**Solutions:**

1. Validate configuration:
   ```bash
   sudo iofog-agent config validate
   ```

2. Check YAML syntax:
   ```bash
   yamllint /etc/iofog-agent/config.yaml
   ```

3. Review configuration file:
   ```bash
   sudo cat /etc/iofog-agent/config.yaml
   ```

4. Restore from backup if needed:
   ```bash
   sudo cp /etc/iofog-agent/config.yaml.backup /etc/iofog-agent/config.yaml
   ```

### Configuration Not Applied

**Symptoms:**
- Changes not taking effect
- Old configuration still active

**Solutions:**

1. Restart agent:
   ```bash
   sudo systemctl restart iofog-agent
   ```

2. Verify configuration loaded:
   ```bash
   sudo iofog-agent config show
   ```

3. Check for configuration errors in logs

## Performance Issues

### High Memory Usage

**Symptoms:**
- Agent using excessive memory
- System running out of memory

**Solutions:**

1. Check memory usage:
   ```bash
   ps aux | grep iofog-agent
   ```

2. Review memory limits in configuration

3. Reduce log level:
   ```yaml
   logLevel: WARN
   ```

4. Restart agent periodically if needed

### High CPU Usage

**Symptoms:**
- Agent consuming high CPU
- System slowdown

**Solutions:**

1. Check CPU usage:
   ```bash
   top -p $(pgrep iofog-agentd)
   ```

2. Profile the agent:
   ```bash
   make profile
   ```

3. Check for resource-intensive operations in logs

4. Optimize configuration

### Slow Startup

**Symptoms:**
- Agent takes long to start
- Timeout errors

**Solutions:**

1. Check startup logs:
   ```bash
   sudo journalctl -u iofog-agent --since "5 minutes ago"
   ```

2. Verify Docker is responsive:
   ```bash
   docker ps
   ```

3. Check network connectivity

4. Review module startup order

## Logging Issues

### No Logs Generated

**Symptoms:**
- Log files not created
- Empty log directory

**Solutions:**

1. Check log directory permissions:
   ```bash
   ls -la /var/log/iofog-agent/
   ```

2. Verify log configuration:
   ```bash
   grep -i log /etc/iofog-agent/config.yaml
   ```

3. Create log directory if missing:
   ```bash
   sudo mkdir -p /var/log/iofog-agent
   sudo chown root:root /var/log/iofog-agent
   ```

### Excessive Logging

**Symptoms:**
- Log files growing too large
- Disk space issues

**Solutions:**

1. Reduce log level:
   ```yaml
   logLevel: WARN
   ```

2. Configure log rotation:
   ```yaml
   logRotation: true
   logMaxSize: 100MB
   logMaxFiles: 5
   ```

3. Clean old logs:
   ```bash
   sudo find /var/log/iofog-agent -name "*.log" -mtime +7 -delete
   ```

## Microservice Issues

### Microservices Not Starting

**Symptoms:**
- Microservices not appearing
- Container creation failures

**Solutions:**

1. Check Process Manager logs:
   ```bash
   sudo journalctl -u iofog-agent | grep -i process
   ```

2. Verify microservice configuration from controller

3. Check Docker resources:
   ```bash
   docker system df
   ```

4. Review container logs:
   ```bash
   docker logs <container-id>
   ```

### Message Routing Issues

**Symptoms:**
- Messages not being delivered
- Routing errors

**Solutions:**

1. Check Message Bus status:
   ```bash
   curl http://localhost:54321/api/v1/status/messagebus
   ```

2. Verify AMQP connection:
   ```bash
   docker ps | grep rabbitmq
   ```

3. Check routing configuration

4. Review message bus logs

## Getting Help

If you're still experiencing issues:

1. **Collect Information:**
   ```bash
   # System info
   uname -a
   docker version
   
   # Agent info
   sudo iofog-agent version
   sudo iofog-agent system status
   
   # Logs
   sudo journalctl -u iofog-agent --since "1 hour ago" > agent-logs.txt
   ```

2. **Check Documentation:**
   - [API Documentation](api.md)
   - [Architecture Documentation](architecture.md)
   - [Deployment Guide](deployment.md)

3. **Report Issues:**
   - GitHub Issues: https://github.com/eclipse-iofog/agent/issues
   - Include logs, configuration (sanitized), and system information

4. **Community Support:**
   - Discussion Forum: https://discuss.iofog.org
   - Slack: #iofog channel
