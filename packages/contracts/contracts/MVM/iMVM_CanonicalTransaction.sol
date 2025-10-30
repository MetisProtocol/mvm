// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Contract Imports */

/* External Imports */

/**
 * @title ICanonicalTransactionChain
 */
interface iMVM_CanonicalTransaction {
    /*********
     * Enums *
     *********/

    enum STAKESTATUS {
        INIT,
        SEQ_SET,
        VERIFIER_SET,
        PAYBACK
    }

    /**********
     * Events *
     **********/

    event VerifierStake(
        address _sender,
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        uint256 _amount
    );

    // default : stakeAmount=0, verified=true, sequencer=true
    // sequencer response for stake: stakeAmount>0, verified=true, sequencer=true
    // verifier response for stake timeout: stakeAmount>0, verified=false, sequencer=false
    event SetBatchTxData(
        address _sender,
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        uint256 _stakeAmount,
        bool _verified,
        bool _sequencer
    );

    event AppendBatchElement(
        uint256 _chainId,
        uint256 _batchIndex,
        uint40 _shouldStartAtElement,
        uint24 _totalElementsToAppend,
        uint256 _txBatchSize,
        uint256 _txBatchTime,
        bytes32 _root
    );

    /***********
     * Structs *
     ***********/

    // locker the same sender for 30 min
    struct TxDataSlice {
        address sender;
        uint256 blockNumber;
        uint256 batchIndex;
        uint256 timestamp;
        bytes txData;
        bool verified;
    }

    struct TxDataRequestStake {
        address sender;
        uint256 blockNumber;
        uint256 batchIndex;
        uint256 timestamp;
        uint256 endtime;
        uint256 amount;
        STAKESTATUS status;
    }

    struct BatchElement {
        uint40 shouldStartAtElement;
        uint24 totalElementsToAppend;
        uint256 txBatchSize;
        uint256 txBatchTime; // sequencer client encode timestamp(ms)
        bytes32 root; // merkle hash root with [hash(txDataBytes + blockNumber)]
        uint256 timestamp; // block timestamp
    }

    /*******************************
     * Authorized Setter Functions *
     *******************************/

    /**
     * Sets address's chain id.
     * @param _address contract address.
     * @param _chainId chain id.
     */
    // function setAddressChainId(address _address, uint256 _chainId) external;

    /**
     * Gets address's chain id.
     */
    // function getAddressChainId(address _address) external view returns (uint256);

    /**
     * Sets the verifier stake base cost of ETH.
     * @param _stakeBaseCost Stake base cost for verifier.
     */
    function setStakeBaseCost(uint256 _stakeBaseCost) external;

    /**
     * Gets the verifier stake base cost of ETH.
     */
    function getStakeBaseCost() external view returns (uint256);

    /**
     * Sets the verifier stake unit cost of ETH.
     * @param _stakeUnitCost Stake cost for verifier.
     */
    function setStakeUnitCost(uint256 _stakeUnitCost) external;

    /**
     * Gets the verifier stake unit cost of ETH for per storage unit.
     */
    function getStakeUnitCost() external view returns (uint256);

    /**
     * Gets the verifier stake cost of ETH by batch index.
     */
    function getStakeCostByBatch(
        uint256 _chainId,
        uint256 _batchIndex
    ) external view returns (uint256);

    /**
     * Sets batch transaction data slice size per submit.
     * @param _size Slice size of batch transaction data.
     */
    function setTxDataSliceSize(uint256 _size) external;

    /**
     * Gets batch transaction data slice size per submit.
     */
    function getTxDataSliceSize() external view returns (uint256);

    /**
     * Sets batch size per batch.
     * @param _size Batch size of batch.
     */
    function setTxBatchSize(uint256 _size) external;

    /**
     * Gets batch size per batch.
     */
    function getTxBatchSize() external view returns (uint256);

    /**
     * Sets slice count per batch transaction data.
     * @param _count Slice count per batch transaction data.
     */
    function setTxDataSliceCount(uint256 _count) external;

    /**
     * Gets slice count per batch transaction data.
     */
    function getTxDataSliceCount() external view returns (uint256);

    /**
     * Sets seconds can submit transaction data after staking.
     * @param _seconds Seconds the Sequencer can sumbit transaction data after verifier staking.
     */
    function setStakeSeqSeconds(uint256 _seconds) external;

    /**
     * Gets seconds can submit transaction data after staking.
     * @return Seconds the Sequencer can sumbit transaction data after verifier staking.
     */
    function getStakeSeqSeconds() external view returns (uint256);

    function isWhiteListed(address _verifier) external view returns (bool);

    // add the verifier to the whitelist
    function setWhiteList(address _verifier, bool _allowed) external;

    // allow everyone to be the verifier
    function disableWhiteList() external;

    /**
     * Allows the sequencer to append a batch of transactions.
     * @dev This function uses a custom encoding scheme for efficiency reasons.
     */
    function appendSequencerBatchByChainId() external;

    /**
     * Sets batch tx data for stake.
     * @param _chainId chain id.
     * @param _batchIndex batch index of CTC.
     * @param _blockNumber slice index.
     * @param _data tx data hex.
     */
    function setBatchTxDataForStake(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data,
        uint256 _leafIndex,
        uint256 _totalLeaves,
        bytes32[] memory _proof
    ) external;

    /**
     * Sets batch tx data for verifier.
     * @param _chainId chain id.
     * @param _batchIndex batch index of CTC.
     * @param _blockNumber slice index.
     * @param _data tx data hex.
     */
    function setBatchTxDataForVerifier(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data
    ) external;

    /**
     * Gets batch tx data.
     * @param _chainId chain id.
     * @param _batchIndex batch index of CTC.
     * @param _blockNumber block number.
     * @return txData
     * @return verified
     */
    function getBatchTxData(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber
    ) external view returns (bytes memory txData, bool verified);

    function checkBatchTxHash(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bytes memory _data
    ) external view returns (bytes32 txHash, bool verified);

    function setBatchTxDataVerified(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber,
        bool _verified
    ) external;

    /**
     * Stake by verifier.
     * @param _chainId chain id.
     * @param _batchIndex batch index of CTC.
     * @param _blockNumber block number.
     */
    function verifierStake(
        uint256 _chainId,
        uint256 _batchIndex,
        uint256 _blockNumber
    ) external payable;

    /**
     * Withdraw stake by verifier.
     * @param _chainId chain id.
     * @param _batchIndex batch index of CTC.
     * @param _blockNumber block number.
     */
    function withdrawStake(uint256 _chainId, uint256 _batchIndex, uint256 _blockNumber) external;
}
