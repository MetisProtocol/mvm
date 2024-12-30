// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Library Imports */
import "../L1/dispute/interfaces/IFaultDisputeGame.sol";
import "contracts/L1/dispute/lib/Types.sol";
import "contracts/L1/dispute/lib/Errors.sol";
import { IBondManager } from "../L1/verification/IBondManager.sol";
import { IChainStorageContainer } from "../L1/rollup/IChainStorageContainer.sol";
import { IMVMStateCommitmentChain } from "../L1/rollup/IMVMStateCommitmentChain.sol";

/* Interface Imports */
import { IStateCommitmentChain } from "../L1/rollup/IStateCommitmentChain.sol";
// import { ICanonicalTransactionChain } from "../L1/rollup/ICanonicalTransactionChain.sol";
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";
import { Lib_MerkleTree } from "../libraries/utils/Lib_MerkleTree.sol";
import { Lib_OVMCodec } from "../libraries/codec/Lib_OVMCodec.sol";
import { Lib_Uint } from "../libraries/utils/Lib_Uint.sol";
import { IDisputeGameFactory } from "../L1/dispute/interfaces/IDisputeGameFactory.sol";

/**
 * @title MVM_StateCommitmentChain
 * @dev The State Commitment Chain (SCC) contract contains a list of proposed state roots which
 * Proposers assert to be a result of each transaction in the Canonical Transaction Chain (CTC).
 * Elements here have a 1:1 correspondence with transactions in the CTC, and should be the unique
 * state root calculated off-chain by applying the canonical transactions one by one.
 *
 * Runtime target: EVM
 */
contract MVM_StateCommitmentChain is IMVMStateCommitmentChain, Lib_AddressResolver {
    /*************
     * Constants *
     *************/

    uint256 public FRAUD_PROOF_WINDOW;
    uint256 public SEQUENCER_PUBLISH_WINDOW;

    uint256 public DEFAULT_CHAINID = 1088;

    string constant public DISPUTE_GAME_FACTORY_NAME = "DisputeGameFactory";

    /*****************
     * Public States *
     *****************/
    // key: chain id
    // value: [8B Timestamp][8B Index]
    mapping(uint256 => bytes16[]) public batchTimes;
    // key: state batch hash
    // value: last L2 block number of the given batch
    mapping(bytes32 => uint256) public batchLastL2BlockNumbers;
    // key: state batch hash
    // value: whether the batch is disputed
    mapping(bytes32 => bool) public disputedBatches;
    // the index of the first disputable batch
    uint256 public firstDisputableBatchIndex;

    /***************
     * Constructor *
     ***************/

    /**
     * @param _libAddressManager Address of the Address Manager.
     */
    constructor(
        address _libAddressManager,
        uint256 _fraudProofWindow,
        uint256 _sequencerPublishWindow
    ) Lib_AddressResolver(_libAddressManager) {
        FRAUD_PROOF_WINDOW = _fraudProofWindow;
        SEQUENCER_PUBLISH_WINDOW = _sequencerPublishWindow;
    }

    function setFraudProofWindow(uint256 window) external {
        require(msg.sender == resolve("METIS_MANAGER"), "not allowed");
        FRAUD_PROOF_WINDOW = window;
    }

    /********************
     * Public Functions *
     ********************/

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function findEarliestDisputableBatch(uint256 _chainId) public view returns (bytes32, uint256) {
        uint256 earliestDisputableTime = block.timestamp - FRAUD_PROOF_WINDOW;
        return _findBatchWithinTimeWindow(_chainId, earliestDisputableTime);
    }

    /**
     * Accesses the batch storage container.
     * @return Reference to the batch storage container.
     */
    function batches() public view returns (IChainStorageContainer) {
        return IChainStorageContainer(resolve("ChainStorageContainer-SCC-batches"));
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function getTotalElements() external view returns (uint256 _totalElements) {
        return getTotalElementsByChainId(DEFAULT_CHAINID);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function getTotalBatches() external view returns (uint256 _totalBatches) {
        return getTotalBatchesByChainId(DEFAULT_CHAINID);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function getLastSequencerTimestamp() external view returns (uint256 _lastSequencerTimestamp) {
        return getLastSequencerTimestampByChainId(DEFAULT_CHAINID);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function appendStateBatch(bytes32[] memory _batch, uint256 _shouldStartAtElement, bytes32 _lastBatchBlockHash, uint256 _lastBatchBlockNumber) external {
        //require (1==0, "don't use");
        string memory proposer = string(
            abi.encodePacked(Lib_Uint.uint2str(DEFAULT_CHAINID), "_MVM_Proposer")
        );
        appendStateBatchByChainId(DEFAULT_CHAINID, _batch, _shouldStartAtElement, proposer, _lastBatchBlockHash, _lastBatchBlockNumber);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function deleteStateBatch(Lib_OVMCodec.ChainBatchHeader memory _batchHeader) external {
        deleteStateBatchByChainId(DEFAULT_CHAINID, _batchHeader);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function verifyStateCommitment(
        bytes32 _element,
        Lib_OVMCodec.ChainBatchHeader memory _batchHeader,
        Lib_OVMCodec.ChainInclusionProof memory _proof
    ) external view returns (bool) {
        return verifyStateCommitmentByChainId(DEFAULT_CHAINID, _element, _batchHeader, _proof);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function insideFraudProofWindow(Lib_OVMCodec.ChainBatchHeader memory _batchHeader)
        public
        view
        returns (bool _inside)
    {
        (uint256 timestamp, , , ) = _decodeExtraData(_batchHeader.extraData);

        require(timestamp != 0, "Batch header timestamp cannot be zero");
        return (timestamp + FRAUD_PROOF_WINDOW) > block.timestamp;
    }

    function insideFraudProofWindowByChainId(
        uint256,
        Lib_OVMCodec.ChainBatchHeader memory _batchHeader
    ) public view override returns (bool _inside) {
        (uint256 timestamp, , , ) = _decodeExtraData(_batchHeader.extraData);

        return _insideFraudProofWindowByChainId(timestamp);
    }

    function isDisputedBatch(bytes32 stateHeaderHash) public view returns (bool) {
        return disputedBatches[stateHeaderHash];
    }

    function saveDisputedBatch(bytes32 stateHeaderHash) public {
        // Grab the game and game data.
        IFaultDisputeGame game = IFaultDisputeGame(msg.sender);
        (GameType gameType, Claim rootClaim, bytes memory extraData) = game.gameData();

        // Grab the verified address of the game based on the game data.
        // slither-disable-next-line unused-return
        (IDisputeGame factoryRegisteredGame,) =
                            IDisputeGameFactory(resolve(DISPUTE_GAME_FACTORY_NAME)).games({ _gameType: gameType, _rootClaim: rootClaim, _extraData: extraData });

        // Must be a valid game.
        if (address(factoryRegisteredGame) != address(game)) revert UnregisteredGame();

        if (disputedBatches[stateHeaderHash]) revert ClaimAlreadyResolved();

        // We only record the disputed batch if the challenger wins.
        if (game.status() == GameStatus.CHALLENGER_WINS) {
            disputedBatches[stateHeaderHash] = true;
        }
    }

    /**********************
     * Internal Functions *
     **********************/

    function _decodeExtraData(bytes memory _extraData) internal pure returns (uint256, address, bytes32, uint256) {
        uint256 timestamp;
        address sequencer;
        bytes32 lastBlockHash;
        uint256 lastBlockNumber;
        if (_extraData.length == 0x40) {
            (timestamp, sequencer) = abi.decode(_extraData, (uint256, address));
        } else if (_extraData.length == 0x80) {
            (timestamp, sequencer, lastBlockHash, lastBlockNumber) = abi.decode(_extraData, (uint256, address, bytes32, uint256));
        } else {
            revert("Invalid extra data length");
        }
        return (timestamp, sequencer, lastBlockHash, lastBlockNumber);
    }

    function _findBatchWithinTimeWindow(uint256 _chainId, uint256 earliestTime) public view returns (bytes32, uint256) {
        bytes16[] storage batchTimesOfChain = batchTimes[_chainId];

        require(batchTimesOfChain.length > 0, "No batch has been appended yet");

        uint256 found = 0;
        uint256 lastTimeIndex = batchTimesOfChain.length - 1;
        uint256 lastElementTime = uint256(uint128(batchTimesOfChain[lastTimeIndex]) >> 64);
        uint256 firstElementTime = uint256(uint128(batchTimesOfChain[firstDisputableBatchIndex]) >> 64);

        require(earliestTime <= lastElementTime, "No batch to dispute");

        if (earliestTime <= firstElementTime) {
            found = 0;
        } else {
            // binary search the batch times to find the closest time of earliestTime,
            // but the time must >= earliestTime
            uint256 left = 0;
            uint256 right = lastTimeIndex;

            while (left < right) {
                uint256 mid = left + (right - left) / 2;
                uint256 time = uint256(uint128(batchTimesOfChain[mid]) >> 64);
                if (time < earliestTime) {
                    left = mid + 1;
                } else {
                    right = mid;
                }
            }
            found = left;
        }

        uint256 batchIndex = uint256(uint64(uint128(batchTimesOfChain[found])));
        bytes32 batchHeaderHash = batches().getByChainId(_chainId, batchIndex);
        return (batchHeaderHash, batchLastL2BlockNumbers[batchHeaderHash]);
    }

    /**
     * Checks whether a given batch is still inside its fraud proof window.
     * @param _timestamp Timestamp of the batch to check.
     * @return _inside Whether or not the batch is inside the fraud proof window.
     */
    function _insideFraudProofWindowByChainId(
       uint256 _timestamp
    ) internal view returns (bool _inside) {
        require(_timestamp != 0, "Batch header timestamp cannot be zero");
        return _timestamp + FRAUD_PROOF_WINDOW > block.timestamp;
    }

    /**
     * Parses the batch context from the extra data.
     * @return Total number of elements submitted.
     * @return Timestamp of the last batch submitted by the sequencer.
     */
    function _getBatchExtraData() internal view returns (uint40, uint40) {
        return _getBatchExtraDataByChainId(DEFAULT_CHAINID);
    }

    /**
     * Encodes the batch context for the extra data.
     * @param _totalElements Total number of elements submitted.
     * @param _lastSequencerTimestamp Timestamp of the last batch submitted by the sequencer.
     * @return Encoded batch context.
     */
    function _makeBatchExtraData(uint40 _totalElements, uint40 _lastSequencerTimestamp)
        internal
        pure
        returns (bytes27)
    {
        bytes27 extraData;
        assembly {
            extraData := _totalElements
            extraData := or(extraData, shl(40, _lastSequencerTimestamp))
            extraData := shl(40, extraData)
        }

        return extraData;
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function getTotalElementsByChainId(uint256 _chainId)
        public
        view
        override
        returns (uint256 _totalElements)
    {
        (uint40 totalElements, ) = _getBatchExtraDataByChainId(_chainId);
        return uint256(totalElements);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function getTotalBatchesByChainId(uint256 _chainId)
        public
        view
        override
        returns (uint256 _totalBatches)
    {
        return batches().lengthByChainId(_chainId);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function getLastSequencerTimestampByChainId(uint256 _chainId)
        public
        view
        override
        returns (uint256 _lastSequencerTimestamp)
    {
        (, uint40 lastSequencerTimestamp) = _getBatchExtraDataByChainId(_chainId);
        return uint256(lastSequencerTimestamp);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function appendStateBatchByChainId(
        uint256 _chainId,
        bytes32[] memory _batch,
        uint256 _shouldStartAtElement,
        string memory _proposer,
        bytes32 _lastBatchBlockHash,
        uint256 _lastBatchBlockNumber
    ) public override {
        // Fail fast in to make sure our batch roots aren't accidentally made fraudulent by the
        // publication of batches by some other user.
        require(
            _shouldStartAtElement == getTotalElementsByChainId(_chainId),
            "Actual batch start index does not match expected start index."
        );

        address proposerAddr = resolve(_proposer);

        // Proposers must have previously staked at the BondManager
        require(
            IBondManager(resolve("BondManager")).isCollateralizedByChainId(
                _chainId,
                msg.sender,
                proposerAddr
            ),
            "Proposer does not have enough collateral posted"
        );

        require(_batch.length > 0, "Cannot submit an empty state batch.");

        // Not check this when submit transaction batch to inbox address
        // require(
        //     getTotalElementsByChainId(_chainId) + _batch.length <=
        //         ICanonicalTransactionChain(resolve("CanonicalTransactionChain"))
        //             .getTotalElementsByChainId(_chainId),
        //     "Number of state roots cannot exceed the number of canonical transactions."
        // );

        // Pass the block's timestamp and the publisher of the data
        // to be used in the fraud proofs
        _appendBatchByChainId(
            _chainId,
            _batch,
            abi.encode(block.timestamp, msg.sender, _lastBatchBlockHash, _lastBatchBlockNumber),
            proposerAddr
        );
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function deleteStateBatchByChainId(
        uint256 _chainId,
        Lib_OVMCodec.ChainBatchHeader memory _batchHeader
    ) public override {
        require(
            msg.sender ==
                resolve(
                    string(abi.encodePacked(Lib_Uint.uint2str(_chainId), "_MVM_FraudVerifier"))
                ),
            "State batches can only be deleted by the MVM_FraudVerifier."
        );

        require(
            insideFraudProofWindow(_batchHeader),
            "State batches can only be deleted within the fraud proof window."
        );

        _deleteBatchByChainId(_chainId, _batchHeader);
    }

    /**
     * @inheritdoc IMVMStateCommitmentChain
     */
    function verifyStateCommitmentByChainId(
        uint256 _chainId,
        bytes32 _element,
        Lib_OVMCodec.ChainBatchHeader memory _batchHeader,
        Lib_OVMCodec.ChainInclusionProof memory _proof
    ) public view override returns (bool) {
        require(_isValidBatchHeaderByChainId(_chainId, _batchHeader), "Invalid batch header.");

        require(
            Lib_MerkleTree.verify(
                _batchHeader.batchRoot,
                _element,
                _proof.index,
                _proof.siblings,
                _batchHeader.batchSize
            ),
            "Invalid inclusion proof."
        );

        return true;
    }

    /**********************
     * Internal Functions *
     **********************/

    /**
     * Parses the batch context from the extra data.
     * @return Total number of elements submitted.
     * @return Timestamp of the last batch submitted by the sequencer.
     */
    function _getBatchExtraDataByChainId(uint256 _chainId) internal view returns (uint40, uint40) {
        bytes27 extraData = batches().getGlobalMetadataByChainId(_chainId);

        uint40 totalElements;
        uint40 lastSequencerTimestamp;
        assembly {
            extraData := shr(40, extraData)
            totalElements := and(
                extraData,
                0x000000000000000000000000000000000000000000000000000000FFFFFFFFFF
            )
            lastSequencerTimestamp := shr(
                40,
                and(extraData, 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0000000000)
            )
        }

        return (totalElements, lastSequencerTimestamp);
    }

    /**
     * Encodes the batch context for the extra data.
     * @param _totalElements Total number of elements submitted.
     * @param _lastSequencerTimestamp Timestamp of the last batch submitted by the sequencer.
     * @return Encoded batch context.
     */
    function _makeBatchExtraDataByChainId(uint40 _totalElements, uint40 _lastSequencerTimestamp)
        internal
        pure
        returns (bytes27)
    {
        bytes27 extraData;
        assembly {
            extraData := _totalElements
            extraData := or(extraData, shl(40, _lastSequencerTimestamp))
            extraData := shl(40, extraData)
        }

        return extraData;
    }

    /**
     * Appends a batch to the chain.
     * @param _batch Elements within the batch.
     * @param _extraData Any extra data to append to the batch.
     */
    function _appendBatchByChainId(
        uint256 _chainId,
        bytes32[] memory _batch,
        bytes memory _extraData,
        address
    ) internal {
        (uint40 totalElements, uint40 lastSequencerTimestamp) = _getBatchExtraDataByChainId(
            _chainId
        );

        lastSequencerTimestamp = uint40(block.timestamp);

        // For efficiency reasons getMerkleRoot modifies the `_batch` argument in place
        // while calculating the root hash therefore any arguments passed to it must not
        // be used again afterwards
        Lib_OVMCodec.ChainBatchHeader memory batchHeader = Lib_OVMCodec.ChainBatchHeader({
            batchIndex: getTotalBatchesByChainId(_chainId),
            batchRoot: Lib_MerkleTree.getMerkleRoot(_batch),
            batchSize: _batch.length,
            prevTotalElements: totalElements,
            extraData: _extraData
        });

        emit StateBatchAppended(
            _chainId,
            batchHeader.batchIndex,
            batchHeader.batchRoot,
            batchHeader.batchSize,
            batchHeader.prevTotalElements,
            batchHeader.extraData
        );

        bytes32 batchHeaderHash = Lib_OVMCodec.hashBatchHeader(batchHeader);

        batches().pushByChainId(
            _chainId,
            batchHeaderHash,
            _makeBatchExtraDataByChainId(
                uint40(batchHeader.prevTotalElements + batchHeader.batchSize),
                lastSequencerTimestamp
            )
        );

        bytes16[] storage batchTimesOfChain = batchTimes[_chainId];
        batchTimesOfChain.push(bytes16(uint128(uint256(lastSequencerTimestamp) << 64) | uint128(batchHeader.batchIndex)));
        batchLastL2BlockNumbers[batchHeaderHash] = batchHeader.prevTotalElements + batchHeader.batchSize;
    }

    /**
     * Removes a batch and all subsequent batches from the chain.
     * @param _batchHeader Header of the batch to remove.
     */
    function _deleteBatchByChainId(
        uint256 _chainId,
        Lib_OVMCodec.ChainBatchHeader memory _batchHeader
    ) internal {
        require(
            _batchHeader.batchIndex < batches().lengthByChainId(_chainId),
            "Invalid batch index."
        );

        require(_isValidBatchHeaderByChainId(_chainId, _batchHeader), "Invalid batch header.");

        // clear the fdg extra data if needed
        if (_batchHeader.extraData.length >= 0x80) {
            (uint256 timestamp, , , ) = abi.decode(_batchHeader.extraData, (uint256, address, bytes32, uint256));
            (bytes32 anchoredBatchHeaderHash, ) = _findBatchWithinTimeWindow(_chainId, timestamp);

            bytes32 batchHeaderHash = Lib_OVMCodec.hashBatchHeader(_batchHeader);
            require(
                batchHeaderHash == anchoredBatchHeaderHash,
                "Anchored batch header does not match the submitted."
            );
            delete batchLastL2BlockNumbers[batchHeaderHash];

            bytes16[] storage batchTimesOfChain = batchTimes[_chainId];
            uint256 batchesToPop = 0;
            for (uint256 i = batchTimesOfChain.length - 1; i >= 0; --i) {
                if (uint64(uint128(batchTimesOfChain[i])) >= _batchHeader.batchIndex) {
                    ++batchesToPop;
                } else {
                    break;
                }
            }
            for (uint256 i = 0; i < batchesToPop; ++i) {
                batchTimesOfChain.pop();
            }
        }

        batches().deleteElementsAfterInclusiveByChainId(
            _chainId,
            _batchHeader.batchIndex,
            _makeBatchExtraDataByChainId(uint40(_batchHeader.prevTotalElements), 0)
        );

        emit StateBatchDeleted(_chainId, _batchHeader.batchIndex, _batchHeader.batchRoot);
    }

    /**
     * Checks that a batch header matches the stored hash for the given index.
     * @param _batchHeader Batch header to validate.
     * @return Whether or not the header matches the stored one.
     */
    function _isValidBatchHeaderByChainId(
        uint256 _chainId,
        Lib_OVMCodec.ChainBatchHeader memory _batchHeader
    ) internal view returns (bool) {
        return
            Lib_OVMCodec.hashBatchHeader(_batchHeader) ==
            batches().getByChainId(_chainId, _batchHeader.batchIndex);
    }
}
