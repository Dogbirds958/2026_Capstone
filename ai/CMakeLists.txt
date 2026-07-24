cmake_minimum_required(VERSION 3.16)
project(simple_classifier_inference LANGUAGES CXX)

set(ONNXRUNTIME_ROOT "" CACHE PATH "Path to the prebuilt ONNX Runtime directory")

if(NOT ONNXRUNTIME_ROOT)
    message(FATAL_ERROR
        "ONNXRUNTIME_ROOT is required. Example: "
        "cmake -DONNXRUNTIME_ROOT=/path/to/third_party/onnxruntime -S ai -B build/ai")
endif()

find_path(ONNXRUNTIME_INCLUDE_DIR
    NAMES onnxruntime_c_api.h onnxruntime_cxx_api.h
    PATHS "${ONNXRUNTIME_ROOT}/include"
    NO_DEFAULT_PATH
    REQUIRED
)

find_library(ONNXRUNTIME_LIBRARY
    NAMES onnxruntime
    PATHS "${ONNXRUNTIME_ROOT}/lib"
    NO_DEFAULT_PATH
    REQUIRED
)

add_library(onnxruntime SHARED IMPORTED GLOBAL)
set_target_properties(onnxruntime PROPERTIES
    IMPORTED_LOCATION "${ONNXRUNTIME_LIBRARY}"
    INTERFACE_INCLUDE_DIRECTORIES "${ONNXRUNTIME_INCLUDE_DIR}"
)

message(STATUS "ONNX Runtime include: ${ONNXRUNTIME_INCLUDE_DIR}")
message(STATUS "ONNX Runtime library: ${ONNXRUNTIME_LIBRARY}")

add_executable(inference-runner inference_runner.cpp)
target_compile_features(inference-runner PRIVATE cxx_std_17)
target_link_libraries(inference-runner PRIVATE onnxruntime)

set_target_properties(inference-runner PROPERTIES
    CXX_STANDARD 17
    CXX_STANDARD_REQUIRED YES
    CXX_EXTENSIONS NO
)

if(MSVC)
    target_compile_options(inference-runner PRIVATE /W4 /permissive-)
else()
    target_compile_options(inference-runner PRIVATE -Wall -Wextra -Wpedantic)
endif()

# Allow the host-side build to run without setting LD_LIBRARY_PATH. The binary
# and shared libraries will be installed separately in the container rootfs.
set_property(TARGET inference-runner PROPERTY BUILD_RPATH "${ONNXRUNTIME_ROOT}/lib")
