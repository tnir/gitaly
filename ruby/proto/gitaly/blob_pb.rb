# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: blob.proto

require 'google/protobuf'

require 'lint_pb'
require 'shared_pb'
Google::Protobuf::DescriptorPool.generated_pool.build do
  add_file("blob.proto", :syntax => :proto3) do
    add_message "gitaly.GetBlobRequest" do
      optional :repository, :message, 1, "gitaly.Repository"
      optional :oid, :string, 2
      optional :limit, :int64, 3
    end
    add_message "gitaly.GetBlobResponse" do
      optional :size, :int64, 1
      optional :data, :bytes, 2
      optional :oid, :string, 3
    end
    add_message "gitaly.GetBlobsRequest" do
      optional :repository, :message, 1, "gitaly.Repository"
      repeated :revision_paths, :message, 2, "gitaly.GetBlobsRequest.RevisionPath"
      optional :limit, :int64, 3
    end
    add_message "gitaly.GetBlobsRequest.RevisionPath" do
      optional :revision, :string, 1
      optional :path, :bytes, 2
    end
    add_message "gitaly.GetBlobsResponse" do
      optional :size, :int64, 1
      optional :data, :bytes, 2
      optional :oid, :string, 3
      optional :is_submodule, :bool, 4
      optional :mode, :int32, 5
      optional :revision, :string, 6
      optional :path, :bytes, 7
      optional :type, :enum, 8, "gitaly.ObjectType"
    end
    add_message "gitaly.LFSPointer" do
      optional :size, :int64, 1
      optional :data, :bytes, 2
      optional :oid, :string, 3
    end
    add_message "gitaly.NewBlobObject" do
      optional :size, :int64, 1
      optional :oid, :string, 2
      optional :path, :bytes, 3
    end
    add_message "gitaly.GetLFSPointersRequest" do
      optional :repository, :message, 1, "gitaly.Repository"
      repeated :blob_ids, :string, 2
    end
    add_message "gitaly.GetLFSPointersResponse" do
      repeated :lfs_pointers, :message, 1, "gitaly.LFSPointer"
    end
    add_message "gitaly.GetAllLFSPointersRequest" do
      optional :repository, :message, 1, "gitaly.Repository"
    end
    add_message "gitaly.GetAllLFSPointersResponse" do
      repeated :lfs_pointers, :message, 1, "gitaly.LFSPointer"
    end
    add_message "gitaly.ListLFSPointersRequest" do
      optional :repository, :message, 1, "gitaly.Repository"
      repeated :revisions, :string, 2
      optional :limit, :int32, 3
    end
    add_message "gitaly.ListLFSPointersResponse" do
      repeated :lfs_pointers, :message, 1, "gitaly.LFSPointer"
    end
    add_message "gitaly.ListAllLFSPointersRequest" do
      optional :repository, :message, 1, "gitaly.Repository"
      optional :limit, :int32, 3
    end
    add_message "gitaly.ListAllLFSPointersResponse" do
      repeated :lfs_pointers, :message, 1, "gitaly.LFSPointer"
    end
  end
end

module Gitaly
  GetBlobRequest = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetBlobRequest").msgclass
  GetBlobResponse = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetBlobResponse").msgclass
  GetBlobsRequest = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetBlobsRequest").msgclass
  GetBlobsRequest::RevisionPath = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetBlobsRequest.RevisionPath").msgclass
  GetBlobsResponse = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetBlobsResponse").msgclass
  LFSPointer = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.LFSPointer").msgclass
  NewBlobObject = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.NewBlobObject").msgclass
  GetLFSPointersRequest = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetLFSPointersRequest").msgclass
  GetLFSPointersResponse = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetLFSPointersResponse").msgclass
  GetAllLFSPointersRequest = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetAllLFSPointersRequest").msgclass
  GetAllLFSPointersResponse = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.GetAllLFSPointersResponse").msgclass
  ListLFSPointersRequest = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.ListLFSPointersRequest").msgclass
  ListLFSPointersResponse = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.ListLFSPointersResponse").msgclass
  ListAllLFSPointersRequest = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.ListAllLFSPointersRequest").msgclass
  ListAllLFSPointersResponse = ::Google::Protobuf::DescriptorPool.generated_pool.lookup("gitaly.ListAllLFSPointersResponse").msgclass
end
