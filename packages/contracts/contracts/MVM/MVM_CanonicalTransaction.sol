// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Library Imports */
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";

/* Interface Imports */
import { iMVM_CanonicalTransaction } from "./iMVM_CanonicalTransaction.sol";
import { ICanonicalTransactionChain } from "../L1/rollup/ICanonicalTransactionChain.sol";
import { IChainStorageContainer } from "../L1/rollup/IChainStorageContainer.sol";
import { StateCommitmentChain } from "../L1/rollup/StateCommitmentChain.sol";
import { Lib_MerkleTree } from "../libraries/utils/Lib_MerkleTree.sol";
import { Lib_Uint } from "../libraries/utils/Lib_Uint.sol";

contract MVM_CanonicalTransaction is iMVM_CanonicalTransaction, Lib_AddressResolver {
    /*************
     * Constants *
     *************/

    string public constant CONFIG_OWNER_KEY = "METIS_MANAGER";

    // lock seconds when begin to submit batch tx data slice
    uint256 public constant TXDATA_SUBMIT_TIMEOUT = 1800;

    /*************
     * Variables *
     *************/

    // submit tx data slice size (in bytes)
    uint256 public txDataSliceSize;
    // stake duration seconds for sequencer submit batch tx data
    uint256 public stakeSeqSeconds;
    // verifier stake base cost for a batch tx data requirement (in ETH)
    uint256 public stakeBaseCost;
    // submit tx data slice count (a whole tx batch)
    uint256 public txDataSliceCount;
    // submit tx batch size (in bytes)
    uint256 public txBatchSize;
    // verifier stake unit cost for a batch tx data requirement (in ETH)
    uint256 public stakeUnitCost;

    bool useWhiteList;

    /***************
     * Queue State *
     ***************/

    // white list
    mapping(address => bool) public whitelist;

    // mapping(address => uint256) private addressChains;

    // verifier stakes statistic
    mapping(address => uint256) private verifierStakes;

    // batch element information for validation queue
    mapping(uint256 => mapping(uint256 => BatchElement)) queueBatchElement;

    // tx data request stake queue
    mapping(uint256 => mapping(uint256 => TxDataRequestStake)) queueTxDataRequestStake;

    // tx data for verification queue
    mapping(uint256 => mapping(uint256 => TxDataSlice)) queueTxData;

    /***************
     * Constructor *
     ***************/

    constructor() Lib_AddressResolver(address(0)) {}

    /**********************
     * Function Modifiers *
     **********************/

    modifier onlyManager() {
        require(
            msg.sender == resolve(CONFIG_OWNER_KEY),
            "MVM_CanonicalTransaction: Function can only be called by the METIS_MANAGER."
        );
        _;
    }

    modifier onlyWhitelisted() {
        require(isWhiteListed(msg.sender), "only whitelisted verifiers can call");
        _;
    }

    /********************
     * Public Functions *
     ********************/
    /**
    receive() external payable {
        // msg.sender
        if (msg.sender == resolve('MVM_DiscountOracle')) {
            uint256 _chainId = getAddressChainId(msg.sender);
            if (_chainId > 0) {
                address _to = resolve(string(abi.encodePacked(Lib_Uint.uint2str(_chainId),"_MVM_Sequencer_Wrapper")));
                if (_to != address(0) && _to != address(this)) {
                    _to.call{value: msg.value}("");
                }
            }
        }
    }

    function setAddressChainId(address _address, uint256 _chainId)  override public onlyManager {
        require(_address != address(0), "address not available");
        require(_chainId > 0, "chainId not available");
        require(addressChains[_address] != _chainId, "no change");
        addressChains[_address] = _chainId;
    }

    function getAddressChainId(address _address) override public view returns (uint256) {
        return addressChains[_address];
    }
    */

    function setStakeBaseCost(uint256 _stakeBaseCost) public override onlyManager {
        // 1e16 = 0.01 ether
        // require(_stakeBaseCost >= 1e16, "stake base cost should gte 1e16");
        stakeBaseCost = _stakeBaseCost;
    }

    function getStakeBaseCost() public view override returns (uint256) {
        return stakeBaseCost;
    }

    function setStakeUnitCost(uint256 _stakeUnitCost) public override onlyManager {
        // 1e16 = 0.01 ether
        stakeUnitCost = _stakeUnitCost;
    }

    function getStakeUnitCost() public view override returns (uint256) {
        return stakeUnitCost;
    }

    function getStakeCostByBatch(
        uint256 _chainId,
        uint256 _batchIndex
    ) public view override returns (uint256) {
        require(stakeBaseCost > 0, "stake base cost not config yet");
        require(
            queueBatchElement[_chainId][_batchIndex].txBatchTime > 0,
            "batch element does not exist"
        );
        return stakeBaseCost + queueBatchElement[_chainId][_batchIndex].txBatchSize * stakeUnitCost;
    }

    function setTxDataSliceSize(uint256 _size) public override onlyManager {
        require(_size > 0, "slice size should gt 0");
        require(_size != txDataSliceSize, "slice size has not changed");
        txDataSliceSize = _size;
    }

    function getTxDataSliceSize() public view override returns (uint256) {
        return txDataSliceSize;
    }

    function setTxDataSliceCount(uint256 _count) public override onlyManager {
        require(_count > 0, "slice count should gt 0");
        require(_count != txDataSliceCount, "slice count has not changed");
        txDataSliceCount = _count;
    }

    function getTxDataSliceCount() public view override returns (uint256) {
        return txDataSliceCount;
    }

    function setTxBatchSize(uint256 _size) public override onlyManager {
        require(_size > 0, "batch size should gt 0");
        require(_size != txBatchSize, "batch size has not changed");
        txBatchSize = _size;
    }

    function getTxBatchSize() public view override returns (uint256) {
        return txBatchSize;
    }

    function setStakeSeqSeconds(uint256 _seconds) public override onlyManager {
        require(_seconds > 0, "seconds should gt 0");
        require(_seconds != stakeSeqSeconds, "seconds has not changed");
        stakeSeqSeconds = _seconds;
    }

    function getStakeSeqSeconds() public view override returns (uint256) {
        return stakeSeqSeconds;
    }

    function isWhiteListed(address _verifier) public view override returns (bool) {
        return !useWhiteList || whitelist[_verifier];
    }

    function setWhiteList(address _verifier, bool _allowed) public override onlyManager {
        whitelist[_verifier] = _allowed;
        useWhiteList = true;
    }

    function disableWhiteList() public override onlyManager {
        useWhiteList = false;
    }

    function appendSequencerBatchByChainId() public override {
        uint256 _chainId;
        uint40 shouldStartAtElement;
        uint24 totalElementsToAppend;
        uint24 numContexts;
        uint256 batchTime;
        uint256 _dataSize;
        uint256 txSize;
        bytes32 root;
        assembly {
            _dataSize := calldatasize()
            _chainId := calldataload(4)
            shouldStartAtElement := shr(216, calldataload(36))
            totalElementsToAppend := shr(232, calldataload(41))
            numContexts := shr(232, calldataload(44))
        }
        require(
            msg.sender ==
                resolve(
                    string(abi.encodePacked(Lib_Uint.uint2str(_chainId), "_MVM_Sequencer_Wrapper"))
                ),
            "Function can only be called by the Sequencer."
        );
        uint256 posTs = 47 + 16 * numContexts;
        if (_dataSize > posTs) {
            // when tx count = 0, there is no hash!
            // string len: [13]{milliseconds}-[1]{0}-[8]{sizeOfData}-[64]{hash}-[64]{root}
            uint256 posTxSize = 7 + posTs;
            uint256 posRoot = 11 + posTs;
            assembly {
                batchTime := shr(204, calldataload(posTs))
                txSize := shr(224, calldataload(posTxSize))
                root := calldataload(posRoot)
            }

            // check batch size
            require(txSize / 2 <= txBatchSize, "size of tx data is too large");
        }

        address ctc = resolve("CanonicalTransactionChain");
        IChainStorageContainer batchesRef = ICanonicalTransactionChain(ctc).batches();
        uint256 batchIndex = batchesRef.lengthByChainId(_chainId);
        {
            // ctc call
            (bool success, bytes memory result) = ctc.call(msg.data);
            if (success == false) {
                assembly {
                    let ptr := mload(0x40)
                    let size := returndatasize()
                    returndatacopy(ptr, 0, size)
                    revert(ptr, size)
                }
            }
        }

        // save
        queueBatchElement[_chainId][batchIndex] = BatchElement({
            shouldStartAtElement: shouldStartAtElement,
            totalElementsToAppend: totalElementsToAppend,
            txBatchSize: txSize,
            txBatchTime: batchTime,
            root: root,
            timestamp: block.timestamp
        });

        emit AppendBatchElement(
            _chainId,
            batchIndex,
            shouldStartAtElement,
            totalElementsToAppend,
            txSize,
            batchTime,
            root
        );
    }

    function setBatchTxDataForStake(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data,
        uint256 _leafIndex,
        uint256 _totalLeaves,
        bytes32[] memory _proof
    ) public override {
        require(
            msg.sender ==
                resolve(
                    string(abi.encodePacked(Lib_Uint.uint2str(_chainId), "_MVM_Sequencer_Wrapper"))
                ),
            "Function can only be called by the Sequencer."
        );
        // check stake
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].timestamp > 0,
            "there is no stake for this block number"
        );
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].batchIndex == _batchIndex,
            "incorrect batch index"
        );
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].status == STAKESTATUS.INIT,
            "not allowed to submit"
        );
        // sequencer can submit at any time
        // require(queueTxDataRequestStake[_chainId][_blockNumber].endtime >= block.timestamp, "can not submit out of sequencer submit protection");

        _setBatchTxData(
            _chainId,
            _batchIndex,
            _blockNumber,
            _data,
            _leafIndex,
            _totalLeaves,
            _proof,
            true
        );

        if (queueTxDataRequestStake[_chainId][_blockNumber].status == STAKESTATUS.INIT) {
            require(
                queueTxDataRequestStake[_chainId][_blockNumber].amount <=
                    verifierStakes[queueTxDataRequestStake[_chainId][_blockNumber].sender],
                "insufficient stake"
            );
            require(
                queueTxDataRequestStake[_chainId][_blockNumber].amount <= address(this).balance,
                "insufficient balance"
            );
            queueTxDataRequestStake[_chainId][_blockNumber].status = STAKESTATUS.SEQ_SET;
            if (queueTxDataRequestStake[_chainId][_blockNumber].amount > 0) {
                verifierStakes[
                    queueTxDataRequestStake[_chainId][_blockNumber].sender
                ] -= queueTxDataRequestStake[_chainId][_blockNumber].amount;
                // transfer from contract to sender ETHER and record
                (bool success, ) = payable(msg.sender).call{
                    value: queueTxDataRequestStake[_chainId][_blockNumber].amount
                }("");
                require(success, "insufficient balance");
                queueTxDataRequestStake[_chainId][_blockNumber].amount = 0;
            }
        }

        emit SetBatchTxData(
            msg.sender,
            _chainId,
            _batchIndex,
            _blockNumber,
            queueTxDataRequestStake[_chainId][_blockNumber].amount,
            true,
            true
        );
    }

    function setBatchTxDataForVerifier(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data
    ) public override {
        require(
            msg.sender !=
                resolve(
                    string(abi.encodePacked(Lib_Uint.uint2str(_chainId), "_MVM_Sequencer_Wrapper"))
                ),
            "Function can not be called by the Sequencer."
        );
        // check stake
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].timestamp > 0,
            "there is no stake for this block number"
        );
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].batchIndex == _batchIndex,
            "incorrect batch index"
        );
        // require(queueTxDataRequestStake[_chainId][_blockNumber].status == STAKESTATUS.INIT, "not allowed to submit");
        // require(queueTxDataRequestStake[_chainId][_blockNumber].sender == msg.sender, "can not submit with other's stake");
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].endtime < block.timestamp,
            "can not submit during sequencer submit protection"
        );
        if (queueTxDataRequestStake[_chainId][_blockNumber].sender != msg.sender) {
            // other verifier can submit in double window times
            require(
                queueTxDataRequestStake[_chainId][_blockNumber].endtime + stakeSeqSeconds <
                    block.timestamp,
                "can not submit during staker submit protection"
            );
        }

        _setBatchTxData(_chainId, _batchIndex, _blockNumber, _data, 0, 0, new bytes32[](0), false);

        if (queueTxDataRequestStake[_chainId][_blockNumber].status == STAKESTATUS.INIT) {
            queueTxDataRequestStake[_chainId][_blockNumber].status = STAKESTATUS.VERIFIER_SET;

            address claimer = queueTxDataRequestStake[_chainId][_blockNumber].sender;
            if (
                queueTxDataRequestStake[_chainId][_blockNumber].amount <= verifierStakes[claimer] &&
                queueTxDataRequestStake[_chainId][_blockNumber].amount > 0
            ) {
                require(
                    queueTxDataRequestStake[_chainId][_blockNumber].amount <= address(this).balance,
                    "insufficient balance"
                );
                verifierStakes[claimer] -= queueTxDataRequestStake[_chainId][_blockNumber].amount;
                // transfer from contract to sender ETHER and record
                (bool success, ) = payable(claimer).call{
                    value: queueTxDataRequestStake[_chainId][_blockNumber].amount
                }("");
                require(success, "insufficient balance");
                queueTxDataRequestStake[_chainId][_blockNumber].amount = 0;
            }
        }

        emit SetBatchTxData(
            msg.sender,
            _chainId,
            _batchIndex,
            _blockNumber,
            queueTxDataRequestStake[_chainId][_blockNumber].amount,
            false,
            false
        );
    }

    function _setBatchTxData(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data,
        uint256 _leafIndex,
        uint256 _totalLeaves,
        bytes32[] memory _proof,
        bool _requireVerify
    ) internal {
        require(_data.length > 0, "empty data");
        // check queue BatchElement
        require(
            queueBatchElement[_chainId][_batchIndex].txBatchTime > 0,
            "batch element does not exist"
        );
        require(
            queueBatchElement[_chainId][_batchIndex].totalElementsToAppend > 0,
            "batch total element to append should not be zero"
        );

        // sequencer protect
        if (queueTxData[_chainId][_blockNumber].timestamp > 0) {
            require(queueTxData[_chainId][_blockNumber].verified == false, "tx data verified");
            if (queueTxData[_chainId][_blockNumber].sender != msg.sender) {
                require(
                    queueTxData[_chainId][_blockNumber].timestamp + TXDATA_SUBMIT_TIMEOUT >
                        block.timestamp,
                    "in submitting"
                );

                // change sumbitter
                queueTxData[_chainId][_blockNumber].sender = msg.sender;
                queueTxData[_chainId][_blockNumber].blockNumber = _blockNumber;
                queueTxData[_chainId][_blockNumber].batchIndex = _batchIndex;
                queueTxData[_chainId][_blockNumber].timestamp = block.timestamp;
                queueTxData[_chainId][_blockNumber].txData = _data;
                queueTxData[_chainId][_blockNumber].verified = false;
            } else {
                queueTxData[_chainId][_blockNumber].txData = _data;
                // verified restore to false
                queueTxData[_chainId][_blockNumber].verified = false;
            }
        } else {
            queueTxData[_chainId][_blockNumber] = TxDataSlice({
                sender: msg.sender,
                blockNumber: _blockNumber,
                batchIndex: _batchIndex,
                timestamp: block.timestamp,
                txData: _data,
                verified: false
            });
        }
        if (_requireVerify) {
            bytes32 currLeaf = keccak256(abi.encodePacked(_blockNumber, _data));
            bool verified = Lib_MerkleTree.verify(
                queueBatchElement[_chainId][_batchIndex].root,
                currLeaf,
                _leafIndex,
                _proof,
                _totalLeaves
            );
            require(verified == true, "tx data verify failed");

            // save verified status
            queueTxData[_chainId][_blockNumber].verified = true;
        }
    }

    function getBatchTxData(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber
    ) external view override returns (bytes memory txData, bool verified) {
        require(queueTxData[_chainId][_blockNumber].timestamp != 0, "tx data does not exist");
        require(
            queueTxData[_chainId][_blockNumber].batchIndex == _batchIndex,
            "incorrect batch index"
        );
        return (
            queueTxData[_chainId][_blockNumber].txData,
            queueTxData[_chainId][_blockNumber].verified
        );
    }

    function checkBatchTxHash(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data
    ) external view override returns (bytes32 txHash, bool verified) {
        require(queueTxData[_chainId][_blockNumber].timestamp != 0, "tx data does not exist");
        require(
            queueTxData[_chainId][_blockNumber].batchIndex == _batchIndex,
            "incorrect batch index"
        );
        return (
            keccak256(abi.encodePacked(_blockNumber, _data)),
            queueTxData[_chainId][_blockNumber].verified
        );
    }

    function setBatchTxDataVerified(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bool _verified
    ) public override onlyManager {
        require(queueTxData[_chainId][_blockNumber].timestamp != 0, "tx data does not exist");
        require(
            queueTxData[_chainId][_blockNumber].batchIndex == _batchIndex,
            "incorrect batch index"
        );
        require(
            queueTxData[_chainId][_blockNumber].verified != _verified,
            "verified status not change"
        );

        queueTxData[_chainId][_blockNumber].verified = _verified;
    }

    function verifierStake(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber
    ) public payable override onlyWhitelisted {
        uint256 _amount = msg.value;
        uint256 stakeCost = getStakeCostByBatch(_chainId, _batchIndex);
        require(stakeBaseCost > 0, "stake base cost not config yet");
        require(stakeCost == _amount, "stake cost incorrect");
        require(stakeSeqSeconds > 0, "sequencer submit seconds not config yet");
        // check queue BatchElement
        require(
            queueBatchElement[_chainId][_batchIndex].txBatchTime > 0,
            "batch element does not exist"
        );
        // check block number in batch range, block number = index + 1
        require(
            queueBatchElement[_chainId][_batchIndex].totalElementsToAppend +
                queueBatchElement[_chainId][_batchIndex].shouldStartAtElement >=
                _blockNumber &&
                queueBatchElement[_chainId][_batchIndex].shouldStartAtElement < _blockNumber,
            "block number is not in this batch"
        );
        if (queueTxDataRequestStake[_chainId][_blockNumber].timestamp > 0) {
            require(
                queueTxDataRequestStake[_chainId][_blockNumber].status == STAKESTATUS.PAYBACK,
                "there is a stake for this batch index"
            );
        }

        //check window
        StateCommitmentChain stateChain = StateCommitmentChain(resolve("StateCommitmentChain"));
        require(
            queueBatchElement[_chainId][_batchIndex].timestamp + stateChain.FRAUD_PROOF_WINDOW() >
                block.timestamp,
            "the batch is outside of the fraud proof window"
        );

        queueTxDataRequestStake[_chainId][_blockNumber] = TxDataRequestStake({
            sender: msg.sender,
            blockNumber: _blockNumber,
            batchIndex: _batchIndex,
            timestamp: block.timestamp,
            endtime: block.timestamp + stakeSeqSeconds,
            amount: _amount,
            status: STAKESTATUS.INIT
        });
        verifierStakes[msg.sender] += _amount;

        emit VerifierStake(msg.sender, _chainId, _batchIndex, _blockNumber, _amount);
    }

    function withdrawStake(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber
    ) public override {
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].timestamp > 0,
            "there is no stake for this batch index"
        );
        require(queueTxDataRequestStake[_chainId][_blockNumber].amount > 0, "stake amount is zero");
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].status == STAKESTATUS.INIT,
            "withdrawals are not allowed"
        );
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].sender == msg.sender,
            "can not withdraw other's stake"
        );
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].endtime < block.timestamp,
            "can not withdraw during submit protection"
        );
        require(
            queueTxDataRequestStake[_chainId][_blockNumber].amount <= verifierStakes[msg.sender],
            "insufficient stake"
        );

        require(
            queueTxDataRequestStake[_chainId][_blockNumber].amount <= address(this).balance,
            "insufficient balance"
        );
        queueTxDataRequestStake[_chainId][_blockNumber].status = STAKESTATUS.PAYBACK;
        verifierStakes[msg.sender] -= queueTxDataRequestStake[_chainId][_blockNumber].amount;
        // transfer from contract to sender ETHER and record
        (bool success, ) = payable(msg.sender).call{
            value: queueTxDataRequestStake[_chainId][_blockNumber].amount
        }("");
        require(success, "insufficient balance");
        queueTxDataRequestStake[_chainId][_blockNumber].amount = 0;
    }
}
