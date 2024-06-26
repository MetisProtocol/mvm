// GENERATED CODE -- DO NOT EDIT!

'use strict';
var grpc = require('grpc');
var disperser_pb = require('./disperser_pb.js');
var common_pb = require('./common_pb.js');

function serialize_disperser_AuthenticatedReply(arg) {
  if (!(arg instanceof disperser_pb.AuthenticatedReply)) {
    throw new Error('Expected argument of type disperser.AuthenticatedReply');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_AuthenticatedReply(buffer_arg) {
  return disperser_pb.AuthenticatedReply.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_AuthenticatedRequest(arg) {
  if (!(arg instanceof disperser_pb.AuthenticatedRequest)) {
    throw new Error('Expected argument of type disperser.AuthenticatedRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_AuthenticatedRequest(buffer_arg) {
  return disperser_pb.AuthenticatedRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_BlobStatusReply(arg) {
  if (!(arg instanceof disperser_pb.BlobStatusReply)) {
    throw new Error('Expected argument of type disperser.BlobStatusReply');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_BlobStatusReply(buffer_arg) {
  return disperser_pb.BlobStatusReply.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_BlobStatusRequest(arg) {
  if (!(arg instanceof disperser_pb.BlobStatusRequest)) {
    throw new Error('Expected argument of type disperser.BlobStatusRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_BlobStatusRequest(buffer_arg) {
  return disperser_pb.BlobStatusRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_DisperseBlobReply(arg) {
  if (!(arg instanceof disperser_pb.DisperseBlobReply)) {
    throw new Error('Expected argument of type disperser.DisperseBlobReply');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_DisperseBlobReply(buffer_arg) {
  return disperser_pb.DisperseBlobReply.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_DisperseBlobRequest(arg) {
  if (!(arg instanceof disperser_pb.DisperseBlobRequest)) {
    throw new Error('Expected argument of type disperser.DisperseBlobRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_DisperseBlobRequest(buffer_arg) {
  return disperser_pb.DisperseBlobRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_RetrieveBlobReply(arg) {
  if (!(arg instanceof disperser_pb.RetrieveBlobReply)) {
    throw new Error('Expected argument of type disperser.RetrieveBlobReply');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_RetrieveBlobReply(buffer_arg) {
  return disperser_pb.RetrieveBlobReply.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_disperser_RetrieveBlobRequest(arg) {
  if (!(arg instanceof disperser_pb.RetrieveBlobRequest)) {
    throw new Error('Expected argument of type disperser.RetrieveBlobRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_disperser_RetrieveBlobRequest(buffer_arg) {
  return disperser_pb.RetrieveBlobRequest.deserializeBinary(new Uint8Array(buffer_arg));
}


// Disperser defines the public APIs for dispersing blobs.
var DisperserService = exports.DisperserService = {
  // This API accepts blob to disperse from clients.
// This executes the dispersal async, i.e. it returns once the request
// is accepted. The client could use GetBlobStatus() API to poll the the
// processing status of the blob.
disperseBlob: {
    path: '/disperser.Disperser/DisperseBlob',
    requestStream: false,
    responseStream: false,
    requestType: disperser_pb.DisperseBlobRequest,
    responseType: disperser_pb.DisperseBlobReply,
    requestSerialize: serialize_disperser_DisperseBlobRequest,
    requestDeserialize: deserialize_disperser_DisperseBlobRequest,
    responseSerialize: serialize_disperser_DisperseBlobReply,
    responseDeserialize: deserialize_disperser_DisperseBlobReply,
  },
  // DisperseBlobAuthenticated is similar to DisperseBlob, except that it requires the
// client to authenticate itself via the AuthenticationData message. The protoco is as follows:
// 1. The client sends a DisperseBlobAuthenticated request with the DisperseBlobRequest message
// 2. The Disperser sends back a BlobAuthHeader message containing information for the client to
//    verify and sign.
// 3. The client verifies the BlobAuthHeader and sends back the signed BlobAuthHeader in an
// 	  AuthenticationData message.
// 4. The Disperser verifies the signature and returns a DisperseBlobReply message.
disperseBlobAuthenticated: {
    path: '/disperser.Disperser/DisperseBlobAuthenticated',
    requestStream: true,
    responseStream: true,
    requestType: disperser_pb.AuthenticatedRequest,
    responseType: disperser_pb.AuthenticatedReply,
    requestSerialize: serialize_disperser_AuthenticatedRequest,
    requestDeserialize: deserialize_disperser_AuthenticatedRequest,
    responseSerialize: serialize_disperser_AuthenticatedReply,
    responseDeserialize: deserialize_disperser_AuthenticatedReply,
  },
  // This API is meant to be polled for the blob status.
getBlobStatus: {
    path: '/disperser.Disperser/GetBlobStatus',
    requestStream: false,
    responseStream: false,
    requestType: disperser_pb.BlobStatusRequest,
    responseType: disperser_pb.BlobStatusReply,
    requestSerialize: serialize_disperser_BlobStatusRequest,
    requestDeserialize: deserialize_disperser_BlobStatusRequest,
    responseSerialize: serialize_disperser_BlobStatusReply,
    responseDeserialize: deserialize_disperser_BlobStatusReply,
  },
  // This retrieves the requested blob from the Disperser's backend.
// This is a more efficient way to retrieve blobs than directly retrieving
// from the DA Nodes (see detail about this approach in
// api/proto/retriever/retriever.proto).
// The blob should have been initially dispersed via this Disperser service
// for this API to work.
retrieveBlob: {
    path: '/disperser.Disperser/RetrieveBlob',
    requestStream: false,
    responseStream: false,
    requestType: disperser_pb.RetrieveBlobRequest,
    responseType: disperser_pb.RetrieveBlobReply,
    requestSerialize: serialize_disperser_RetrieveBlobRequest,
    requestDeserialize: deserialize_disperser_RetrieveBlobRequest,
    responseSerialize: serialize_disperser_RetrieveBlobReply,
    responseDeserialize: deserialize_disperser_RetrieveBlobReply,
  },
};

exports.DisperserClient = grpc.makeGenericClientConstructor(DisperserService);
