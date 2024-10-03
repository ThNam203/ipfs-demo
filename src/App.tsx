"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import { UploadCloud, File, Trash2, Download } from "lucide-react";
import { toast } from "./components/ui/use-toast";
import { Separator } from "./components/ui/separator";
import { ScrollArea } from "./components/ui/scrollarea";

type FileItem = {
    name: string;
    size: number;
    type: string;
    file: File;
};

type UploadedFile = {
    filename: string;
    size: number;
    type: string;
    cid: string;
};

const serverIPv4 = "13.215.163.6"

export default function Component() {
    const [isUploading, setIsUploading] = useState(false);
    const [files, setFiles] = useState<FileItem[]>([]);
    const webSocket = useRef<WebSocket | null>(null);
    const [uploadedFiles, setUploadedFiles] = useState<UploadedFile[]>([]);
    const [dragActive, setDragActive] = useState(false);

    useEffect(() => {
        const fetchData = async () => {
            try {

                const response = await fetch("http://" + serverIPv4 + ":8000/files");
                if (response.ok) {
                    try {
                        const data = await response.json();
                        if (data != null || data.length !== 0) setUploadedFiles(data);
                    } catch (_) {}
                } else {
                    toast({
                        title: "Failed to fetch files",
                        description: "An error occurred while fetching the files.",
                    });
                }
          } catch (e) {
            console.log(e)
          }
        };

        fetchData();
    }, []);

    useEffect(() => {
        webSocket.current = new WebSocket("ws://" + serverIPv4 + ":8000/socket");

        webSocket.current.onopen = () => {
            console.log("WebSocket connection established.");
        };

        webSocket.current.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                setUploadedFiles((prevFiles) => [
                    ...prevFiles,
                    {
                        filename: message.filename,
                        size: message.size,
                        type: message.type,
                        cid: message.cid,
                    },
                ]);
            } catch (error) {
                console.error("Failed to parse message:", error);
            }
        };
    }, [webSocket]);

    const handleDrag = (e: React.DragEvent) => {
        e.preventDefault();
        e.stopPropagation();
        if (e.type === "dragenter" || e.type === "dragover") {
            setDragActive(true);
        } else if (e.type === "dragleave") {
            setDragActive(false);
        }
    };

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault();
        e.stopPropagation();
        setDragActive(false);
        if (e.dataTransfer.files && e.dataTransfer.files[0]) {
            handleFiles(e.dataTransfer.files);
        }
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        e.preventDefault();
        if (e.target.files && e.target.files[0]) {
            handleFiles(e.target.files);
        }
    };

    const handleFiles = (fileList: FileList) => {
        const newFiles = Array.from(fileList).map((file) => ({
            name: file.name,
            size: file.size,
            type: file.type,
            file: file,
        }));

        // Check for duplicates by file name and file size
        setFiles((prevFiles) => {
            const filteredNewFiles = newFiles.filter(
                (newFile) =>
                    !prevFiles.some(
                        (file) =>
                            file.name === newFile.name &&
                            file.size === newFile.size
                    )
            );

            if (filteredNewFiles.length < newFiles.length) {
                toast({
                    title: "Duplicate Files",
                    description:
                        "Some files were already added and were not re-added.",
                });
            }

            return [...prevFiles, ...filteredNewFiles];
        });
    };

    const removeFile = (fileName: string) => {
        setFiles((prevFiles) =>
            prevFiles.filter((file) => file.name !== fileName)
        );
    };

    const formatFileSize = (bytes: number) => {
        if (bytes < 1024) return bytes + " bytes";
        else if (bytes < 1048576) return (bytes / 1024).toFixed(1) + " KB";
        else if (bytes < 1073741824)
            return (bytes / 1048576).toFixed(1) + " MB";
        else return (bytes / 1073741824).toFixed(1) + " GB";
    };

    const handleUpload = async () => {
        if (files.length === 0) {
            toast({
                title: "No Files Selected",
                description: "Please select files before uploading.",
            });
            return;
        }

        const formData = new FormData();

        // Append each file to the formData
        files.forEach((fileItem) => {
            formData.append("files", fileItem.file); // 'files' is the field name for file upload
        });

        try {
            setIsUploading(true);
            const response = await fetch("http://" + serverIPv4 + ":8000/upload", {
                method: "POST",
                body: formData,
            });

            if (response.ok) {
                toast({
                    title: "Upload Successful",
                    description: `Uploaded ${files.length} file(s) successfully.`,
                });

                setFiles([]);
            } else {
                throw new Error("Upload failed");
            }
        } catch (error) {
            toast({
                title: "Upload Failed",
                description: "An error occurred while uploading the files.",
            });
            console.error("Error during file upload:", error);
        } finally {
            setIsUploading(false);
        }
    };

    return (
        <div className="flex h-[600px] w-full max-w-6xl mx-auto border rounded-lg overflow-hidden">
            <div className="flex-1 p-6 overflow-auto">
                <h2 className="text-2xl font-bold mb-6">File Sharing</h2>
                <div
                    className={`border-2 border-dashed rounded-lg p-8 text-center ${
                        dragActive
                            ? "border-primary bg-primary/10"
                            : "border-gray-300"
                    }`}
                    onDragEnter={handleDrag}
                    onDragLeave={handleDrag}
                    onDragOver={handleDrag}
                    onDrop={handleDrop}
                >
                    <UploadCloud className="mx-auto h-12 w-12 text-gray-400" />
                    <p className="mt-2 text-sm text-gray-600">
                        Drag and drop your files here, or click to select files
                    </p>
                    <Input
                        id="file-upload"
                        type="file"
                        multiple
                        className="hidden"
                        onChange={handleChange}
                    />
                    <Button asChild className="mt-4">
                        <label htmlFor="file-upload">Select Files</label>
                    </Button>
                </div>
                {files.length > 0 && (
                    <div className="mt-8">
                        <h3 className="text-lg font-semibold mb-4">
                            Selected Files
                        </h3>
                        <ul className="space-y-4 mb-6">
                            {files.map((file, index) => (
                                <li
                                    key={index}
                                    className="flex items-center justify-between p-4 bg-gray-100 rounded-lg"
                                >
                                    <div className="flex items-center">
                                        <File className="h-6 w-6 mr-3 text-blue-500" />
                                        <div>
                                            <p className="font-medium">
                                                {file.name}
                                            </p>
                                            <p className="text-sm text-gray-500">
                                                {formatFileSize(file.size)}
                                            </p>
                                        </div>
                                    </div>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={() => removeFile(file.name)}
                                    >
                                        <Trash2 className="h-5 w-5" />
                                        <span className="sr-only">Remove</span>
                                    </Button>
                                </li>
                            ))}
                        </ul>
                        <Button
                            disabled={isUploading}
                            onClick={handleUpload}
                            className="w-full"
                        >
                            Upload {files.length} file
                            {files.length !== 1 ? "s" : ""}
                        </Button>
                    </div>
                )}
            </div>
            <Separator orientation="vertical" />
            <div className="w-80 p-6 bg-gray-50">
                <h3 className="text-lg font-semibold mb-4">Uploaded Files</h3>
                <ScrollArea className="h-[calc(100%-2rem)]">
                    <ul className="space-y-4">
                        {uploadedFiles.map((file, index) => (
                            <li
                                key={index}
                                className="flex items-center justify-between p-4 bg-white rounded-lg shadow-sm"
                            >
                                <div className="flex items-center">
                                    <div>
                                        <p className="font-medium max-w-[200px]">
                                            {file.filename}
                                        </p>
                                        <p className="text-xs text-gray-500">
                                            {formatFileSize(file.size)}
                                        </p>
                                    </div>
                                </div>
                                <Button
                                    variant="ghost"
                                    size="icon"
                                    onClick={() => {}}
                                >
                                    <a href={`http://${serverIPv4}:8080${file.cid}`} download={file.filename}><Download className="h-5 w-5"/></a>
                                    <span className="sr-only">Download</span>
                                </Button>
                            </li>
                        ))}
                    </ul>
                </ScrollArea>
            </div>
        </div>
    );
}
