// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Library Imports */
import { Lib_Buffer } from "../../libraries/utils/Lib_Buffer.sol";
import { Lib_AddressResolver } from "../../libraries/resolver/Lib_AddressResolver.sol";

/* Interface Imports */
import { IChainStorageContainer } from "./IChainStorageContainer.sol";

/**
 * @title ChainStorageContainer
 * @dev The Chain Storage Container provides its owner contract with read, write and delete
 * functionality. This provides gas efficiency gains by enabling it to overwrite storage slots which
 * can no longer be used in a fraud proof due to the fraud window having passed, and the associated
 * chain state or transactions being finalized.
 * Three distinct Chain Storage Containers will be deployed on Layer 1:
 * 1. Stores transaction batches for the Canonical Transaction Chain
 * 2. Stores queued transactions for the Canonical Transaction Chain
 * 3. Stores chain state batches for the State Commitment Chain
 *
 * Runtime target: EVM
 */
contract ChainStorageContainer is IChainStorageContainer, Lib_AddressResolver {
    /*************
     * Libraries *
     *************/

    using Lib_Buffer for Lib_Buffer.Buffer;

    /**************
     *  constant  *
     **************/
    uint256 constant public DEFAULT_CHAINID = 1088;
    
    
    /*************
     * Variables *
     *************/

    string public owner;
    mapping(uint256=>Lib_Buffer.Buffer) internal buffers;

    /***************
     * Constructor *
     ***************/

    /**
     * @param _libAddressManager Address of the Address Manager.
     * @param _owner Name of the contract that owns this container (will be resolved later).
     */
    constructor(address _libAddressManager, string memory _owner)
        Lib_AddressResolver(_libAddressManager)
    {
        owner = _owner;
    }

    /**********************
     * Function Modifiers *
     **********************/

    modifier onlyOwner() {
        require(
            msg.sender == resolve(owner),
            "ChainStorageContainer: Function can only be called by the owner."
        );
        _;
    }

    /********************
     * Public Functions *
     ********************/

    /**
     * @inheritdoc IChainStorageContainer
     */
    function setGlobalMetadata(bytes27 _globalMetadata) public onlyOwner {
        return setGlobalMetadataByChainId(DEFAULT_CHAINID,_globalMetadata);
    }
    
    function setByChainId(
        uint256 _chainId,
        uint256 _index,
        bytes32 _object
    )
        override
        public
        onlyOwner
    {
        buffers[_chainId].set(_index, _object);
    }
    
    /**
     * @inheritdoc IChainStorageContainer
     */
    function setGlobalMetadataByChainId(
        uint256 _chainId,
        bytes27 _globalMetadata
    )
        override
        public
        onlyOwner
    {
        return buffers[_chainId].setExtraData(_globalMetadata);
    }
    
    /**
     * @inheritdoc IChainStorageContainer
     */
    function getGlobalMetadata() public view returns (bytes27) {
        return getGlobalMetadataByChainId(DEFAULT_CHAINID);
    }
    
    function getGlobalMetadataByChainId(uint256 _chainId)
        override
        public
        view
        returns (
            bytes27
        )
    {
        return buffers[_chainId].getExtraData();
    }

    /**
     * @inheritdoc IChainStorageContainer
     */
    function length() public view returns (uint256) {
        return lengthByChainId(DEFAULT_CHAINID);
    }
    
    function lengthByChainId(uint256 _chainId)
        override
        public
        view
        returns (
            uint256
        )
    {
        return uint256(buffers[_chainId].getLength());
    }

    /**
     * @inheritdoc IChainStorageContainer
     */
    function push(bytes32 _object) public onlyOwner {
        pushByChainId(DEFAULT_CHAINID,_object);
    }
    
    function pushByChainId(
        uint256 _chainId,
        bytes32 _object
    )
        override
        public
        onlyOwner
    {
        buffers[_chainId].push(_object);
    }

    /**
     * @inheritdoc IChainStorageContainer
     */
    function push(bytes32 _object, bytes27 _globalMetadata) public onlyOwner {
        pushByChainId(DEFAULT_CHAINID,_object,_globalMetadata);
    }
    
    function pushByChainId(
        uint256 _chainId,
        bytes32 _object,
        bytes27 _globalMetadata
    )
        override
        public
        onlyOwner
    {
        buffers[_chainId].push(_object, _globalMetadata);
    }

    /**
     * @inheritdoc IChainStorageContainer
     */
    function get(uint256 _index) public view returns (bytes32) {
        return getByChainId(DEFAULT_CHAINID,_index);
    }
    
    function getByChainId(
        uint256 _chainId,
        uint256 _index
    )
        override
        public
        view
        returns (
            bytes32
        )
    {
        return buffers[_chainId].get(uint40(_index));
    }

    /**
     * @inheritdoc IChainStorageContainer
     */
    function deleteElementsAfterInclusive(uint256 _index) public onlyOwner {
        deleteElementsAfterInclusiveByChainId(DEFAULT_CHAINID,_index);
    }
    
    function deleteElementsAfterInclusiveByChainId(
        uint256 _chainId,
        uint256 _index
    )
        override
        public
        onlyOwner
    {
        buffers[_chainId].deleteElementsAfterInclusive(
            uint40(_index)
        );
    }

    /**
     * @inheritdoc IChainStorageContainer
     */
    function deleteElementsAfterInclusive(uint256 _index, bytes27 _globalMetadata)
        public
        onlyOwner
    {
        deleteElementsAfterInclusiveByChainId(DEFAULT_CHAINID,_index,_globalMetadata);
    }
    
    function deleteElementsAfterInclusiveByChainId(
        uint256 _chainId,
        uint256 _index,
        bytes27 _globalMetadata
    )
        override
        public
        onlyOwner
    {
        buffers[_chainId].deleteElementsAfterInclusive(
            uint40(_index),
            _globalMetadata
        );
    }
}
