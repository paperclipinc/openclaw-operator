# Custom AI Providers

This guide covers common patterns for connecting OpenClaw to self-hosted or alternative AI providers.

## Ollama as a Sidecar

> **Tip:** Since v0.10.0, the operator has first-class Ollama support via `spec.ollama`.
> The manual sidecar approach below still works but the built-in integration handles
> model pulling, GPU resources, and volume setup automatically.
> See the [README](index.md#ollama-sidecar) for details.

Run Ollama alongside OpenClaw in the same pod. This is the simplest option when you want local model inference without network hops.

Provider configuration is done entirely through environment variables. Set `OPENAI_BASE_URL` to point OpenClaw at your custom provider endpoint:

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: local-llm
spec:
  sidecars:
    - name: ollama
      image: ollama/ollama:latest
      ports:
        - containerPort: 11434
          protocol: TCP
      volumeMounts:
        - name: ollama-models
          mountPath: /root/.ollama

  sidecarVolumes:
    - name: ollama-models
      emptyDir:
        sizeLimit: 20Gi

  env:
    - name: OPENAI_API_KEY
      value: "not-needed"
    - name: OPENAI_BASE_URL
      value: "http://localhost:11434/v1"

  resources:
    requests:
      cpu: "2"
      memory: 8Gi
    limits:
      cpu: "8"
      memory: 16Gi

  security:
    networkPolicy:
      enabled: true
      # No egress needed for local-only inference
```

### GPU Support

For GPU-accelerated Ollama, add resource limits and node selectors:

```yaml
spec:
  sidecars:
    - name: ollama
      image: ollama/ollama:latest
      resources:
        limits:
          nvidia.com/gpu: "1"
      volumeMounts:
        - name: ollama-models
          mountPath: /root/.ollama

  availability:
    nodeSelector:
      gpu: "true"
    tolerations:
      - key: nvidia.com/gpu
        operator: Exists
        effect: NoSchedule
```

## Ollama as an External Service

When Ollama runs as a separate Deployment or on bare metal, point OpenClaw to it via environment variables and allow egress to the service.

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: external-ollama
spec:
  env:
    - name: OPENAI_API_KEY
      value: "not-needed"
    - name: OPENAI_BASE_URL
      value: "http://ollama.inference.svc:11434/v1"

  security:
    networkPolicy:
      enabled: true
      additionalEgress:
        - to:
            - namespaceSelector:
                matchLabels:
                  kubernetes.io/metadata.name: inference
              podSelector:
                matchLabels:
                  app: ollama
          ports:
            - protocol: TCP
              port: 11434
```

## vLLM via OpenAI-Compatible API

[vLLM](https://docs.vllm.ai/) exposes an OpenAI-compatible API. Configure it the same way as Ollama:

```yaml
apiVersion: openclaw.rocks/v1alpha1
kind: OpenClawInstance
metadata:
  name: vllm-instance
spec:
  env:
    - name: OPENAI_API_KEY
      value: "not-needed"
    - name: OPENAI_BASE_URL
      value: "http://vllm.inference.svc:8000/v1"

  security:
    networkPolicy:
      enabled: true
      additionalEgress:
        - to:
            - namespaceSelector:
                matchLabels:
                  kubernetes.io/metadata.name: inference
              podSelector:
                matchLabels:
                  app: vllm
          ports:
            - protocol: TCP
              port: 8000
```

## NetworkPolicy Considerations

The default NetworkPolicy allows egress only on port 443 (HTTPS) and port 53 (DNS). When using custom providers on non-standard ports, you must add egress rules:

| Provider Setup        | Port  | Solution                              |
|-----------------------|-------|---------------------------------------|
| Ollama sidecar        | 11434 | No egress needed (localhost)          |
| Ollama external       | 11434 | `additionalEgress` with pod selector  |
| vLLM external         | 8000  | `additionalEgress` with pod selector  |
| Custom HTTPS endpoint | 443   | Already allowed by default            |
| Custom non-443 HTTPS  | 8443  | `additionalEgress` with CIDR or selector |
