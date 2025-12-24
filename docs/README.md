# Project Documentation

This directory contains detailed design documents, performance analysis, and architectural decisions for `go-psrpcore`.

## Design & Architecture

- [Development Journey](development-journey.md)  
  Chronicles the development history, challenges encountered (like the OutOfProcess transport framing), and lessons learned.

- [Endianness Verification](endianness-verification.md)  
  Details the verification of byte order assumptions (Little-Endian vs Big-Endian) across the protocol layers.

## Security & Implementation

- [SecureString Findings (Issue #6)](issue-6-securestring-findings.md)  
  Analysis of the SecureString serialization format and the requirements for session key exchange.

- [Session Key Implementation Guide](session-key-implementation-guide.md)  
  Guide for implementing the required crypto components for SecureString support (RSA key exchange, AES-256-CBC).

## Performance

- [Baseline Performance](BASELINE_PERFORMANCE.md)  
  Performance benchmarks and baseline metrics for the library.
