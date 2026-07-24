#include <onnxruntime_cxx_api.h>

#include <algorithm>
#include <chrono>
#include <charconv>
#include <cstdint>
#include <filesystem>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <limits>
#include <stdexcept>
#include <string>
#include <vector>

namespace {

struct Arguments {
    std::string model_path;
    std::string input_path;
    std::size_t repeat = 1;
};

Arguments parse_arguments(int argc, char* argv[]) {
    Arguments arguments;

    for (int index = 1; index < argc; ++index) {
        const std::string option = argv[index];
        if ((option == "--model" || option == "--input" || option == "--repeat") && index + 1 >= argc) {
            throw std::runtime_error("missing value for " + option);
        }

        if (option == "--model") {
            arguments.model_path = argv[++index];
        } else if (option == "--input") {
            arguments.input_path = argv[++index];
        } else if (option == "--repeat") {
            const std::string value = argv[++index];
            std::size_t repeat = 0;
            const auto parse_result = std::from_chars(value.data(), value.data() + value.size(), repeat);
            if (parse_result.ec != std::errc{} || parse_result.ptr != value.data() + value.size() ||
                repeat == 0 || repeat > 1000000) {
                throw std::runtime_error("--repeat must be an integer between 1 and 1000000");
            }
            arguments.repeat = repeat;
        } else {
            throw std::runtime_error("unknown argument: " + option);
        }
    }

    if (arguments.model_path.empty() || arguments.input_path.empty()) {
        throw std::runtime_error("usage: inference-runner --model MODEL_PATH --input INPUT_PATH");
    }
    return arguments;
}

void require_regular_file(const std::string& path, const char* description) {
    std::error_code error;
    if (!std::filesystem::is_regular_file(path, error)) {
        throw std::runtime_error(std::string(description) + " file not found: " + path);
    }
}

std::size_t element_count(const std::vector<int64_t>& shape) {
    std::size_t count = 1;
    for (const int64_t dimension : shape) {
        if (dimension <= 0) {
            throw std::runtime_error("dynamic or invalid model input shape is not supported");
        }
        const auto unsigned_dimension = static_cast<std::size_t>(dimension);
        if (count > std::numeric_limits<std::size_t>::max() / unsigned_dimension) {
            throw std::runtime_error("model input shape is too large");
        }
        count *= unsigned_dimension;
    }
    return count;
}

std::vector<float> read_float_input(const std::string& path, std::size_t expected_elements) {
    std::ifstream input(path, std::ios::binary | std::ios::ate);
    if (!input) {
        throw std::runtime_error("failed to open input file: " + path);
    }

    const std::streamsize byte_count = input.tellg();
    const std::size_t expected_bytes = expected_elements * sizeof(float);
    if (byte_count < 0 || static_cast<std::size_t>(byte_count) != expected_bytes) {
        throw std::runtime_error(
            "input size mismatch: expected " + std::to_string(expected_bytes) +
            " bytes, got " + std::to_string(byte_count < 0 ? 0 : byte_count));
    }

    std::vector<float> values(expected_elements);
    input.seekg(0, std::ios::beg);
    if (!input.read(reinterpret_cast<char*>(values.data()), byte_count)) {
        throw std::runtime_error("failed to read input file: " + path);
    }
    return values;
}

double elapsed_milliseconds(std::chrono::steady_clock::time_point start,
                            std::chrono::steady_clock::time_point end) {
    return std::chrono::duration<double, std::milli>(end - start).count();
}

}  // namespace

int main(int argc, char* argv[]) {
    const auto process_start = std::chrono::steady_clock::now();
    try {
        const Arguments arguments = parse_arguments(argc, argv);
        require_regular_file(arguments.model_path, "model");
        require_regular_file(arguments.input_path, "input");

        Ort::Env environment(ORT_LOGGING_LEVEL_WARNING, "inference-runner");
        Ort::SessionOptions session_options;
        session_options.SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);

        const auto model_load_start = std::chrono::steady_clock::now();
        Ort::Session session(nullptr);
        try {
            session = Ort::Session(environment, arguments.model_path.c_str(), session_options);
        } catch (const Ort::Exception& error) {
            throw std::runtime_error(std::string("ONNX Runtime session creation failed: ") + error.what());
        }
        const auto model_load_end = std::chrono::steady_clock::now();

        if (session.GetInputCount() != 1 || session.GetOutputCount() != 1) {
            throw std::runtime_error("only models with exactly one input and one output are supported");
        }

        Ort::AllocatorWithDefaultOptions allocator;
        const auto input_name = session.GetInputNameAllocated(0, allocator);
        const auto output_name = session.GetOutputNameAllocated(0, allocator);

        // ConstTensorTypeAndShapeInfo borrows from TypeInfo, so keep its owner alive.
        const auto input_type_info = session.GetInputTypeInfo(0);
        const auto input_info = input_type_info.GetTensorTypeAndShapeInfo();
        if (input_info.GetElementType() != ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
            throw std::runtime_error("input tensor type mismatch: expected float32");
        }
        const std::vector<int64_t> input_shape = input_info.GetShape();
        const std::size_t input_elements = element_count(input_shape);
        std::vector<float> input_values = read_float_input(arguments.input_path, input_elements);

        const Ort::MemoryInfo memory_info =
            Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
        Ort::Value input_tensor = Ort::Value::CreateTensor<float>(
            memory_info,
            input_values.data(),
            input_values.size(),
            input_shape.data(),
            input_shape.size());

        const char* input_names[] = {input_name.get()};
        const char* output_names[] = {output_name.get()};

        std::vector<Ort::Value> outputs;
        std::vector<double> inference_times;
        inference_times.reserve(arguments.repeat);
        for (std::size_t iteration = 0; iteration < arguments.repeat; ++iteration) {
            const auto inference_start = std::chrono::steady_clock::now();
            try {
                outputs = session.Run(
                    Ort::RunOptions{nullptr}, input_names, &input_tensor, 1, output_names, 1);
            } catch (const Ort::Exception& error) {
                throw std::runtime_error(
                    "inference failed at iteration " + std::to_string(iteration + 1) + ": " + error.what());
            }
            const auto inference_end = std::chrono::steady_clock::now();
            inference_times.push_back(elapsed_milliseconds(inference_start, inference_end));
        }

        if (outputs.size() != 1 || !outputs[0].IsTensor()) {
            throw std::runtime_error("output is not a tensor");
        }
        const auto output_info = outputs[0].GetTensorTypeAndShapeInfo();
        if (output_info.GetElementType() != ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
            throw std::runtime_error("output tensor type mismatch: expected float32");
        }

        const std::size_t output_elements = output_info.GetElementCount();
        if (output_elements == 0) {
            throw std::runtime_error("output tensor is empty");
        }
        const float* logits = outputs[0].GetTensorData<float>();
        const std::size_t predicted_class = static_cast<std::size_t>(
            std::max_element(logits, logits + output_elements) - logits);

        const auto process_end = std::chrono::steady_clock::now();
        const auto [minimum_time, maximum_time] =
            std::minmax_element(inference_times.begin(), inference_times.end());
        double total_inference_time = 0.0;
        for (const double inference_time : inference_times) {
            total_inference_time += inference_time;
        }
        const double average_inference_time =
            total_inference_time / static_cast<double>(inference_times.size());

        std::cout << std::fixed << std::setprecision(6)
                  << "{\"inference\":{"
                  << "\"model_load_ms\":" << elapsed_milliseconds(model_load_start, model_load_end) << ','
                  << "\"repeat\":" << arguments.repeat << ','
                  << "\"inference_ms\":" << average_inference_time << ','
                  << "\"inference_avg_ms\":" << average_inference_time << ','
                  << "\"inference_min_ms\":" << *minimum_time << ','
                  << "\"inference_max_ms\":" << *maximum_time << ','
                  << "\"inference_runs_ms\":[";
        for (std::size_t index = 0; index < inference_times.size(); ++index) {
            if (index != 0) {
                std::cout << ',';
            }
            std::cout << inference_times[index];
        }
        std::cout << "],"
                  << "\"runner_total_ms\":" << elapsed_milliseconds(process_start, process_end)
                  << "},\"result\":{\"class\":" << predicted_class << ",\"logits\":[";
        for (std::size_t index = 0; index < output_elements; ++index) {
            if (index != 0) {
                std::cout << ',';
            }
            std::cout << logits[index];
        }
        std::cout << "]}}\n";
        return 0;
    } catch (const Ort::Exception& error) {
        std::cerr << "error: ONNX Runtime error: " << error.what() << '\n';
    } catch (const std::exception& error) {
        std::cerr << "error: " << error.what() << '\n';
    }
    return 1;
}
