의료 AI 추론용 경량 컨테이너 런타임
프로젝트 소개

본 프로젝트는 의료 AI 추론 과정에서 발생할 수 있는 개인정보 및 민감 의료정보(PHI)의 노출 위험을 줄이기 위해 개발한 경량 컨테이너 런타임입니다.

기존의 일반 실행 환경 대신 Linux Namespace, RootFS, Cgroup 기반의 격리 환경을 제공하여 의료 AI 모델이 안전하게 실행될 수 있도록 설계하였습니다.

프로젝트 목표
의료 데이터 보호를 위한 격리 실행 환경 제공
경량 컨테이너 런타임 구현
AI 추론 환경과 컨테이너 기술의 결합
의료기관 내부망(Intranet) 환경 적용 가능성 검증
시스템 구조
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
주요 기능
구현 완료

BusyBox 기반 RootFS 구축

Go 기반 컨테이너 런타임 구현

PID Namespace 기반 프로세스 격리

chroot 기반 파일시스템 격리

BusyBox Shell 실행

개발 예정

PHI 마스킹 모듈

AI 추론 모듈 통합

Cgroup 기반 자원 제한

네트워크 접근 제어

결과 검증 및 로그 관리

GPU 기반 추론 지원
