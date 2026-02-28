# CHStone Go Test Suite Description

## Overview

The CHStone Go version is a benchmark suite for testing Go language High-Level Synthesis (HLS) tools. This suite contains 12 test programs covering multiple application domains including arithmetic operations, media processing, security encryption, and microprocessors.

## List of Test Programs

### 1. Arithmetic Operations

#### DFADD - Double-precision Floating-point Addition
- **File Location**: `dfadd/main.go`
- **Test Content**: Implementation of double-precision floating-point addition
- **Algorithm Source**: SoftFloat library
- **Key Function**: `float64_add` - Implements addition of two double-precision floating-point numbers
- **Test Data**: Predefined floating-point pairs, including boundary values and special values

#### DFMUL - Double-precision Floating-point Multiplication
- **File Location**: `dfmul/main.go`
- **Test Content**: Implementation of double-precision floating-point multiplication
- **Algorithm Source**: SoftFloat library
- **Key Function**: `float64_mul` - Implements multiplication of two double-precision floating-point numbers
- **Test Data**: Multiple sets of floating-point multiplication test cases

#### DFDIV - Double-precision Floating-point Division
- **File Location**: `dfdiv/main.go`
- **Test Content**: Implementation of double-precision floating-point division
- **Algorithm Source**: SoftFloat library
- **Key Function**: `float64_div` - Implements division of two double-precision floating-point numbers
- **Test Data**: Test cases including division by zero and overflow scenarios

#### DFSIN - Sine Function
- **File Location**: `dfsin/main.go`
- **Test Content**: Calculation of sine function for double-precision floating-point numbers
- **Algorithm Source**: Developed by CHStone group, based on SoftFloat
- **Key Function**: `local_sin` - Calculates sine values using Taylor series expansion
- **Test Data**: Multiple angle sine calculation tests

### 2. Media Processing

#### ADPCM - Adaptive Differential Pulse Code Modulation
- **File Location**: `adpcm/main.go`
- **Test Content**: Implementation of ADPCM audio codec
- **Algorithm Source**: SNU real-time benchmarks
- **Key Functions**: 
  - `adpcm_decoder` - ADPCM decoding
  - `adpcm_encoder` - ADPCM encoding
- **Test Data**: Predefined audio sample data

#### GSM - Linear Predictive Coding Analysis
- **File Location**: `gsm/main.go`
- **Test Content**: Linear Predictive Coding (LPC) analysis for Global System for Mobile Communications (GSM)
- **Algorithm Source**: MediaBench
- **Key Function**: `Gsm_LPC_Analysis` - Performs LPC analysis
- **Test Data**: Speech signal sample data

#### MOTION - MPEG-2 Motion Vector Decoding
- **File Location**: `motion/main.go`
- **Test Content**: Motion vector decoding for MPEG-2 video coding
- **Algorithm Source**: MediaBench
- **Key Function**: `motion_vectors` - Decodes motion vectors
- **Test Data**: Simulated video frame data

### 3. Security Encryption

#### AES - Advanced Encryption Standard
- **File Location**: `aes/main.go`
- **Test Content**: Implementation of AES encryption algorithm
- **Algorithm Source**: AILab
- **Key Functions**: 
  - `encrypt` - AES encryption
  - `decrypt` - AES decryption
- **Test Data**: Standard AES test vectors

#### BLOWFISH - Data Encryption Standard
- **File Location**: `blowfish/main.go`
- **Test Content**: Blowfish symmetric encryption algorithm
- **Algorithm Source**: MiBench
- **Key Functions**: 
  - `Blowfish_Encrypt` - Blowfish encryption
  - `Blowfish_Decrypt` - Blowfish decryption
- **Test Data**: Standard Blowfish test vectors

#### SHA - Secure Hash Algorithm
- **File Location**: `sha/main.go`
- **Test Content**: SHA-1 hash algorithm implementation
- **Algorithm Source**: MiBench
- **Key Function**: `sha_stream` - Calculates SHA-1 hash for data streams
- **Test Data**: Predefined test string data

### 4. Microprocessor

#### MIPS - Simplified MIPS Processor
- **File Location**: `mips/main.go`
- **Test Content**: Simplified MIPS processor simulation
- **Algorithm Source**: Developed by CHStone group
- **Key Function**: `mips` - Executes MIPS instructions
- **Test Content**: Executes preloaded MIPS programs

## Test Suite Features

1. **Self-contained test vectors**: Each test program contains predefined input data and expected outputs, no external files required
2. **No external dependencies**: No other external libraries needed except standard library
3. **Cross-platform**: Can be compiled and run on any platform that supports Go
4. **HLS-friendly**: Can be used for Go-to-HLS tool testing after appropriate modifications

## File Structure

```
CHStone/
├── README                      # Original C version description
├── Go_Test_Suite_Description.md # This file (Chinese version)
├── Go_Test_Suite_Description_EN.md # English version
├── Go_HLS_Analysis_Report.md   # HLS suitability analysis report
├── dfadd/                      # Double-precision floating-point addition
│   └── main.go
├── dfmul/                      # Double-precision floating-point multiplication
│   └── main.go
├── dfdiv/                      # Double-precision floating-point division
│   └── main.go
├── dfsin/                      # Sine function
│   └── main.go
├── adpcm/                      # ADPCM codec
│   └── main.go
├── gsm/                        # GSM LPC analysis
│   └── main.go
├── motion/                     # MPEG-2 motion vectors
│   └── main.go
├── aes/                        # AES encryption
│   └── main.go
├── blowfish/                   # Blowfish encryption
│   └── main.go
├── sha/                        # SHA hash
│   └── main.go
├── mips/                       # MIPS processor
│   └── main.go
└── common/                     # Common utility functions
    └── main.go
```

## Citation

If you use the CHStone Go version in your paper, please cite the original CHStone paper:

```
Yuko Hara, Hiroyuki Tomiyama, Shinya Honda and Hiroaki Takada,
"Proposal and Quantitative Analysis of the CHStone Benchmark Program Suite 
for Practical C-based High-level Synthesis",
Journal of Information Processing, Vol. 17, pp.242-254, (2009).
```

