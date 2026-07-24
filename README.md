# 의료 AI 추론용 경량 컨테이너 런타임

## 프로젝트 소개

본 프로젝트는 의료 AI 추론 과정에서 발생할 수 있는 개인정보 및 민감 의료정보(PHI)의 노출 위험을 줄이기 위해 개발한 경량 컨테이너 런타임입니다.

기존의 일반 실행 환경 대신 Linux Namespace, RootFS, Cgroup 기반의 격리 환경을 제공하여 의료 AI 모델이 안전하게 실행될 수 있도록 설계하였습니다.


## 프로젝트 목표

- 의료 데이터 보호를 위한 격리 실행 환경 제공
- 경량 컨테이너 런타임 구현
- AI 추론 환경과 컨테이너 기술의 결합
- 의료기관 내부망(Intranet) 환경 적용 가능성 검증


## 시스템 구조

```text
User Input
    │
    ▼
PHI Masking
    │
    ▼
Go Runtime
    │
    ▼
Namespace + RootFS + Cgroup
    │
    ▼
AI Inference
    │
    ▼
Result Validation
    │
    ▼
User Output
```

## 디렉터리 구조

```text
.
├── main.go                          # 컨테이너 런타임 핵심 코드
├── main_test.go                     # Go 런타임 단위 테스트
├── go.mod                           # Go 모듈 설정
├── config.example.json              # 런타임 설정 예시
│
├── ai/                              # ONNX 모델 및 추론 애플리케이션
│   ├── inference_runner.cpp         # ONNX Runtime 기반 C++ 추론 실행기
│   ├── prepare_model.py             # 테스트 모델 생성 및 검증
│   ├── CMakeLists.txt               # C++ 빌드 설정
│   ├── requirements.txt             # Python 패키지 목록
│   └── README.md                    # AI 워크로드 설명
│
├── model-store/                     # 런타임에 마운트할 모델 저장소
│   └── simple-classifier/
│       └── 1.0.0/
│           ├── model.onnx           # 테스트용 ONNX 모델
│           ├── input.bin            # 테스트용 입력 데이터
│           ├── labels.json          # 출력 클래스 정보
│           └── manifest.json        # 모델 정보 및 무결성 해시
│
├── scripts/
│   └── copy_runtime_dependencies.sh # 공유 라이브러리 복사 스크립트
│
├── tests/                           # 런타임 및 격리 기능 테스트
│   ├── test_common.sh               # 테스트 공통 설정
│   ├── test_host_inference.sh       # 호스트 추론 테스트
│   ├── test_chroot_inference.sh     # chroot 추론 테스트
│   ├── test_runtime_inference.sh    # 전체 런타임 추론 테스트
│   ├── test_model_readonly.sh       # 모델 읽기 전용 마운트 테스트
│   ├── test_missing_model.sh        # 모델 누락 및 무결성 테스트
│   ├── test_cpu_limit.sh            # CPU 제한 테스트
│   ├── test_memory_limit.sh         # 메모리 제한 테스트
│   └── test_cgroup_limits.sh        # cgroup 제한 종합 테스트
│
├── setup_build_tools.sh             # CMake 빌드 도구 준비
├── setup_onnxruntime_cpp.sh         # ONNX Runtime C++ SDK 설치
├── setup_onnx_model.sh              # 테스트용 ONNX 모델 생성
└── setup_busybox_rootfs.sh          # 컨테이너 rootfs 구성
```

다음 항목은 설치 또는 빌드 과정에서 생성되므로 Git으로 관리하지 않습니다.

```text
build/                               # CMake 빌드 결과
busybox-rootfs/                      # 컨테이너 rootfs
third_party/onnxruntime/             # 다운로드한 ONNX Runtime SDK
.venv-build-tools/                   # 빌드 도구용 Python 가상환경
.venv-onnx-export/                   # 모델 생성용 Python 가상환경
mini-container                       # 빌드된 Go 런타임 실행 파일
config.json                          # 사용자별 로컬 실행 설정
```

## 주요 기능

### 구현 완료

- [x] BusyBox 기반 RootFS 구축
- [x] Go 기반 컨테이너 런타임 구현
- [x] PID Namespace 기반 프로세스 격리
- [x] chroot 기반 파일시스템 격리
- [x] BusyBox Shell 실행

### 개발 예정
- [ ] PHI 마스킹 모듈
- [ ] AI 추론 모듈 통합
- [ ] Cgroup 기반 자원 제한
- [ ] 네트워크 접근 제어
- [ ] 결과 검증 및 로그 관리
- [ ] GPU 기반 추론 지원

